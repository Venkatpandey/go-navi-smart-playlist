package lyrics

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"go-navi-smart-playlist/internal/config"
	"go-navi-smart-playlist/internal/model"
)

type scanner interface {
	StartScan(ctx context.Context) error
}

type Provider interface {
	Find(ctx context.Context, track model.Track) (Result, error)
}

type Result struct {
	Synced string
	Plain  string
}

type Runner struct {
	provider Provider
	scanner  scanner
	cfg      config.LyricsConfig
	logger   *log.Logger
	dryRun   bool
}

type Stats struct {
	Checked int
	Written int
	Skipped int
	Failed  int
}

type writeRequest struct {
	track model.Track
	path  string
	text  string
}

func NewRunner(provider Provider, scanner scanner, cfg config.LyricsConfig, logger *log.Logger, dryRun bool) *Runner {
	return &Runner{
		provider: provider,
		scanner:  scanner,
		cfg:      cfg,
		logger:   logger,
		dryRun:   dryRun,
	}
}

func (r *Runner) Run(ctx context.Context, tracks []model.Track) (Stats, error) {
	if !r.cfg.Enabled {
		return Stats{}, nil
	}
	if r.provider == nil {
		return Stats{}, errors.New("lyrics provider is required")
	}

	fetchWorkers := max(r.cfg.FetchWorkers, 1)
	writeWorkers := max(r.cfg.WriteWorkers, 1)
	trackJobs := make(chan model.Track)
	writeJobs := make(chan writeRequest)

	var stats Stats
	var wrote atomic.Bool
	var checked int64
	var written int64
	var skipped int64
	var failed int64
	var fetchWG sync.WaitGroup
	var writeWG sync.WaitGroup
	var errMu sync.Mutex
	var firstErr error
	recordErr := func(err error) {
		errMu.Lock()
		defer errMu.Unlock()
		if firstErr == nil {
			firstErr = err
		}
	}

	for range writeWorkers {
		writeWG.Add(1)
		go func() {
			defer writeWG.Done()
			for job := range writeJobs {
				if err := r.writeSidecar(job); err != nil {
					if errors.Is(err, errSkipped) {
						atomic.AddInt64(&skipped, 1)
						continue
					}
					atomic.AddInt64(&failed, 1)
					recordErr(err)
					continue
				}

				wrote.Store(true)
				atomic.AddInt64(&written, 1)
			}
		}()
	}

	for range fetchWorkers {
		fetchWG.Add(1)
		go func() {
			defer fetchWG.Done()
			for track := range trackJobs {
				atomic.AddInt64(&checked, 1)
				if err := r.processTrack(ctx, track, writeJobs); err != nil {
					if errors.Is(err, errSkipped) {
						atomic.AddInt64(&skipped, 1)
						continue
					}

					atomic.AddInt64(&failed, 1)
					recordErr(err)
				}
			}
		}()
	}

sendTracks:
	for _, track := range tracks {
		select {
		case <-ctx.Done():
			break sendTracks
		case trackJobs <- track:
		}
	}
	close(trackJobs)
	fetchWG.Wait()
	close(writeJobs)
	writeWG.Wait()

	stats = Stats{
		Checked: int(atomic.LoadInt64(&checked)),
		Written: int(atomic.LoadInt64(&written)),
		Skipped: int(atomic.LoadInt64(&skipped)),
		Failed:  int(atomic.LoadInt64(&failed)),
	}

	if wrote.Load() && r.cfg.TriggerScan && !r.dryRun && r.scanner != nil {
		if err := r.scanner.StartScan(ctx); err != nil {
			return stats, fmt.Errorf("start Navidrome scan: %w", err)
		}
		r.logger.Printf("triggered Navidrome scan after lyrics update")
	}

	if firstErr != nil {
		return stats, firstErr
	}

	r.logger.Printf("lyrics job complete: checked=%d written=%d skipped=%d failed=%d", stats.Checked, stats.Written, stats.Skipped, stats.Failed)
	return stats, nil
}

var errSkipped = errors.New("lyrics skipped")

func (r *Runner) processTrack(ctx context.Context, track model.Track, writeJobs chan<- writeRequest) error {
	if strings.TrimSpace(track.Path) == "" || strings.TrimSpace(track.Title) == "" || strings.TrimSpace(track.Artist) == "" {
		return errSkipped
	}

	audioPath, err := r.mapPath(track.Path)
	if err != nil {
		return err
	}

	base := strings.TrimSuffix(audioPath, filepath.Ext(audioPath))
	if !r.cfg.Overwrite && hasExistingSidecar(base) {
		return errSkipped
	}

	result, err := r.provider.Find(ctx, track)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return errSkipped
		}
		return fmt.Errorf("find lyrics for %q by %q: %w", track.Title, track.Artist, err)
	}

	path, text, ok := chooseSidecar(base, result)
	if !ok {
		return errSkipped
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case writeJobs <- writeRequest{track: track, path: path, text: text}:
		return nil
	}
}

func (r *Runner) mapPath(path string) (string, error) {
	cleanPath := filepath.Clean(strings.TrimSpace(path))
	if r.cfg.PathFrom == "" {
		return cleanPath, nil
	}

	from := filepath.Clean(r.cfg.PathFrom)
	to := filepath.Clean(r.cfg.PathTo)
	if cleanPath == from {
		return to, nil
	}

	prefix := from + string(os.PathSeparator)
	if !strings.HasPrefix(cleanPath, prefix) {
		return "", fmt.Errorf("track path %q does not match LYRICS_PATH_PREFIX_FROM %q", cleanPath, from)
	}

	return filepath.Join(to, strings.TrimPrefix(cleanPath, prefix)), nil
}

func (r *Runner) writeSidecar(job writeRequest) error {
	if strings.TrimSpace(job.text) == "" {
		return errSkipped
	}

	if r.dryRun {
		r.logger.Printf("dry-run lyrics write %q for %q by %q", job.path, job.track.Title, job.track.Artist)
		return nil
	}

	if r.cfg.Overwrite {
		if err := removeAlternateSidecar(job.path); err != nil {
			return err
		}
	}

	if err := os.WriteFile(job.path, []byte(strings.TrimSpace(job.text)+"\n"), 0o644); err != nil {
		return fmt.Errorf("write lyrics sidecar %q: %w", job.path, err)
	}

	r.logger.Printf("wrote lyrics sidecar %q", job.path)
	return nil
}

func hasExistingSidecar(base string) bool {
	if _, err := os.Stat(base + ".lrc"); err == nil {
		return true
	}
	if _, err := os.Stat(base + ".txt"); err == nil {
		return true
	}

	return false
}

func removeAlternateSidecar(path string) error {
	base := strings.TrimSuffix(path, filepath.Ext(path))
	var alternate string
	switch filepath.Ext(path) {
	case ".lrc":
		alternate = base + ".txt"
	case ".txt":
		alternate = base + ".lrc"
	default:
		return nil
	}

	if err := os.Remove(alternate); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove alternate lyrics sidecar %q: %w", alternate, err)
	}

	return nil
}

func chooseSidecar(base string, result Result) (string, string, bool) {
	if strings.TrimSpace(result.Synced) != "" {
		return base + ".lrc", result.Synced, true
	}
	if strings.TrimSpace(result.Plain) != "" {
		return base + ".txt", result.Plain, true
	}

	return "", "", false
}

func max(value, fallback int) int {
	if value > 0 {
		return value
	}

	return fallback
}
