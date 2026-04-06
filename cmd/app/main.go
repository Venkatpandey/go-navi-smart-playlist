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
	"go-navi-smart-playlist/internal/navidrome"
	"go-navi-smart-playlist/internal/playlist"
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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	runOnce := func() {
		runCtx, cancel := context.WithTimeout(ctx, cfg.RunTimeout)
		defer cancel()

		if err := run(runCtx, trackCollector, generator, writer, logger); err != nil {
			if errors.Is(err, context.Canceled) {
				logger.Printf("run canceled: %v", err)
				return
			}

			logger.Printf("run failed: %v", err)
		}
	}

	logger.Printf("starting smart playlist job")
	runOnce()

	ticker := time.NewTicker(24 * time.Hour)
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
	generator *playlist.Generator,
	writer *playlist.Writer,
	logger *log.Logger,
) error {
	tracks, err := trackCollector.Collect(ctx)
	if err != nil {
		return err
	}

	logger.Printf("collected %d tracks", len(tracks))

	playlists := generator.Generate(tracks, time.Now().UTC())
	for _, definition := range playlists {
		if err := writer.Upsert(ctx, definition.Name, definition.Tracks); err != nil {
			return err
		}
	}

	logger.Printf("playlist refresh complete")
	return nil
}
