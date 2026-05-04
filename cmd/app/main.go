package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"go-navi-smart-playlist/internal/collector"
	"go-navi-smart-playlist/internal/config"
	"go-navi-smart-playlist/internal/features"
	"go-navi-smart-playlist/internal/navidrome"
	"go-navi-smart-playlist/internal/playlist"
	"go-navi-smart-playlist/internal/state"
)

type userDiscoverer interface {
	Login(ctx context.Context, username, password string) (string, error)
	DiscoverUsers(ctx context.Context, token string) ([]navidrome.DiscoveredUser, error)
}

type userRefresher func(ctx context.Context, cfg config.Config, user config.UserCredential, stateFile string, logger *log.Logger) error

func main() {
	logger := log.New(os.Stdout, "", log.LstdFlags|log.LUTC)

	cfg, err := config.Load()
	if err != nil {
		logger.Fatalf("load config: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	runOnce := func() {
		var err error
		if cfg.MultiUserEnabled {
			err = runMultiUser(ctx, cfg, logger, navidrome.NewNativeClient(cfg, logger), config.LoadUserCredentials, refreshUser)
		} else {
			err = refreshUser(ctx, cfg, config.UserCredential{
				Username: cfg.Username,
				Password: cfg.Password,
			}, cfg.StateFileForUser(cfg.Username), logger)
		}
		if err != nil {
			if errors.Is(err, context.Canceled) {
				logger.Printf("run canceled: %v", err)
				return
			}

			logger.Printf("run failed: %v", err)
		}
	}

	logger.Printf("starting smart playlist job")
	runOnce()

	ticker := time.NewTicker(7 * 24 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Printf("shutting down")
			return
		case <-ticker.C:
			runOnce()
		}
	}
}

func refreshUser(
	ctx context.Context,
	cfg config.Config,
	user config.UserCredential,
	stateFile string,
	logger *log.Logger,
) error {
	runCtx, cancel := context.WithTimeout(ctx, cfg.RunTimeout)
	defer cancel()

	client := navidrome.NewClientWithCredentials(cfg, user.Username, user.Password, logger)
	trackCollector := collector.NewWithPageSize(client, cfg.AlbumPageSize, logger)
	writer := playlist.NewWriter(client, logger, cfg.DryRun)
	generator := playlist.NewGenerator(cfg, logger)
	featureBuilder := features.NewBuilder(logger)
	stateStore := state.NewStore(stateFile, cfg.EnableState, logger)

	return runPlaylistRefresh(runCtx, trackCollector, featureBuilder, generator, writer, stateStore, logger)
}

func runMultiUser(
	ctx context.Context,
	cfg config.Config,
	logger *log.Logger,
	discoverer userDiscoverer,
	loadCredentials func(string) ([]config.UserCredential, error),
	refresh userRefresher,
) error {
	users, err := loadCredentials(cfg.MultiUserConfigFile)
	if err != nil {
		return err
	}

	discoveryCtx, cancel := context.WithTimeout(ctx, cfg.RunTimeout)
	defer cancel()

	token, err := discoverer.Login(discoveryCtx, cfg.AdminUsername, cfg.AdminPassword)
	if err != nil {
		return fmt.Errorf("native admin login failed: %w", err)
	}

	discovered, err := discoverer.DiscoverUsers(discoveryCtx, token)
	if err != nil {
		return fmt.Errorf("native user discovery failed: %w", err)
	}

	credentials := make(map[string]config.UserCredential, len(users))
	for _, user := range users {
		if !user.IsEnabled() {
			continue
		}
		credentials[strings.ToLower(user.Username)] = user
	}

	matched := make(map[string]struct{}, len(credentials))
	successCount := 0
	failureCount := 0

	for _, discoveredUser := range discovered {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if discoveredUser.Enabled != nil && !*discoveredUser.Enabled {
			logger.Printf("skipping disabled user %q discovered via native API", discoveredUser.Username)
			continue
		}

		userLogger := prefixedLogger(logger, discoveredUser.Username)
		credential, ok := credentials[strings.ToLower(discoveredUser.Username)]
		if !ok {
			userLogger.Printf("warning: discovered user has no enabled credentials entry, skipping")
			failureCount++
			continue
		}

		matched[strings.ToLower(discoveredUser.Username)] = struct{}{}
		if err := refresh(ctx, cfg, credential, cfg.StateFileForUser(credential.Username), userLogger); err != nil {
			userLogger.Printf("refresh failed: %v", err)
			failureCount++
			continue
		}

		successCount++
	}

	for _, user := range users {
		if _, ok := matched[strings.ToLower(user.Username)]; ok {
			continue
		}
		if !user.IsEnabled() {
			continue
		}

		logger.Printf("warning: credentials entry %q not found in discovered Navidrome users, ignoring", user.Username)
	}

	logger.Printf("multi-user refresh complete: success=%d failed=%d", successCount, failureCount)
	if failureCount > 0 {
		return fmt.Errorf("multi-user refresh incomplete: success=%d failed=%d", successCount, failureCount)
	}

	return nil
}

func runPlaylistRefresh(
	ctx context.Context,
	trackCollector *collector.Collector,
	featureBuilder *features.Builder,
	generator *playlist.Generator,
	writer *playlist.Writer,
	stateStore *state.Store,
	logger *log.Logger,
) error {
	previousState, err := stateStore.Load()
	if err != nil {
		logger.Printf("state load warning: %v", err)
	}

	tracks, err := trackCollector.Collect(ctx)
	if err != nil {
		return err
	}

	logger.Printf("collected %d tracks", len(tracks))

	now := time.Now().UTC()
	dataset := featureBuilder.Build(tracks, previousState, now)
	playlists := generator.Generate(dataset, previousState, now)
	for _, definition := range playlists {
		if err := writer.Upsert(ctx, definition.Name, definition.Tracks); err != nil {
			return err
		}
	}

	if err := stateStore.Save(buildHistoryState(dataset, playlists, previousState, now)); err != nil {
		logger.Printf("state save warning: %v", err)
	}

	logger.Printf("playlist refresh complete")
	return nil
}

func prefixedLogger(parent *log.Logger, username string) *log.Logger {
	return log.New(parent.Writer(), parent.Prefix()+"["+username+"] ", parent.Flags())
}

func buildHistoryState(
	dataset features.Dataset,
	playlists []playlist.Definition,
	previous *state.HistoryState,
	now time.Time,
) *state.HistoryState {
	result := state.NewHistoryState()
	result.UpdatedAt = now

	for _, item := range dataset.Items {
		previousSeenCount := 0
		if previous != nil {
			if snapshot, ok := previous.Tracks[item.Track.ID]; ok {
				previousSeenCount = snapshot.SeenCount
			}
		}

		result.Tracks[item.Track.ID] = state.TrackSnapshot{
			ID:         item.Track.ID,
			PlayCount:  item.Track.PlayCount,
			LastPlayed: item.Track.LastPlayed,
			Created:    item.Track.Created,
			Artist:     item.Track.Artist,
			Album:      item.Track.Album,
			SeenCount:  previousSeenCount + 1,
			LastSeenAt: now,
			Derived: state.DerivedFeatureSnapshot{
				PlayCountPercentile: item.PlayCountPercentile,
				DaysSinceLastPlayed: item.DaysSinceLastPlayed,
				DaysSinceAdded:      item.DaysSinceAdded,
				PlayCountDelta:      item.PlayCountDelta,
				RecencyTrend:        item.RecencyTrendScore,
				RepeatFatigue:       item.RepeatFatigueScore,
				ArtistSaturation:    item.ArtistSaturation,
				AlbumSaturation:     item.AlbumSaturation,
				NoveltyScore:        item.NoveltyScore,
				StabilityScore:      item.StabilityScore,
				Vector:              item.SimilarityVector,
			},
		}
	}

	for _, definition := range playlists {
		trackIDs := make([]string, 0, len(definition.Tracks))
		for _, track := range definition.Tracks {
			trackIDs = append(trackIDs, track.ID)
		}

		result.Playlists[definition.Name] = state.PlaylistSnapshot{TrackIDs: trackIDs}
	}

	return result
}
