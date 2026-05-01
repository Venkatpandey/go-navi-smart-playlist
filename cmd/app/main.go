package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go-navi-smart-playlist/internal/collector"
	"go-navi-smart-playlist/internal/config"
	"go-navi-smart-playlist/internal/features"
	"go-navi-smart-playlist/internal/lyrics"
	"go-navi-smart-playlist/internal/navidrome"
	"go-navi-smart-playlist/internal/playlist"
	"go-navi-smart-playlist/internal/state"
)

func main() {
	logger := log.New(os.Stdout, "", log.LstdFlags|log.LUTC)

	cfg, err := config.Load()
	if err != nil {
		logger.Fatalf("load config: %v", err)
	}

	client := navidrome.NewClient(cfg, logger)
	trackCollector := collector.NewWithPageSize(client, cfg.AlbumPageSize, logger)
	writer := playlist.NewWriter(client, logger, cfg.DryRun)
	generator := playlist.NewGenerator(cfg, logger)
	featureBuilder := features.NewBuilder(logger)
	stateStore := state.NewStore(cfg.StateFile, cfg.EnableState, logger)
	lyricsRunner := lyrics.NewRunner(lyrics.NewLRCLIBProvider(), client, cfg.Lyrics, logger, cfg.DryRun)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	runOnce := func() {
		runCtx, cancel := context.WithTimeout(ctx, cfg.RunTimeout)
		defer cancel()

		if err := run(runCtx, trackCollector, featureBuilder, generator, writer, stateStore, lyricsRunner, logger); err != nil {
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

func run(
	ctx context.Context,
	trackCollector *collector.Collector,
	featureBuilder *features.Builder,
	generator *playlist.Generator,
	writer *playlist.Writer,
	stateStore *state.Store,
	lyricsRunner *lyrics.Runner,
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

	if _, err := lyricsRunner.Run(ctx, tracks); err != nil {
		logger.Printf("lyrics update warning: %v", err)
	}

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
