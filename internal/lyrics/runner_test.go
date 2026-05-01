package lyrics

import (
	"context"
	"errors"
	"log"
	"os"
	"path/filepath"
	"testing"

	"go-navi-smart-playlist/internal/config"
	"go-navi-smart-playlist/internal/model"
)

func TestChooseSidecarPrefersSyncedLyrics(t *testing.T) {
	path, text, ok := chooseSidecar("/music/song", Result{Synced: "[00:01.00]line", Plain: "line"})
	if !ok {
		t.Fatalf("expected lyrics")
	}
	if path != "/music/song.lrc" {
		t.Fatalf("expected .lrc path, got %q", path)
	}
	if text != "[00:01.00]line" {
		t.Fatalf("expected synced lyrics, got %q", text)
	}
}

func TestChooseSidecarFallsBackToPlainLyrics(t *testing.T) {
	path, text, ok := chooseSidecar("/music/song", Result{Plain: "line"})
	if !ok {
		t.Fatalf("expected lyrics")
	}
	if path != "/music/song.txt" {
		t.Fatalf("expected .txt path, got %q", path)
	}
	if text != "line" {
		t.Fatalf("expected plain lyrics, got %q", text)
	}
}

func TestRunnerMapsPathAndWritesSidecar(t *testing.T) {
	dir := t.TempDir()
	hostPath := "/host/music/Artist/Album/song.flac"
	containerPath := filepath.Join(dir, "Artist", "Album", "song.flac")
	if err := os.MkdirAll(filepath.Dir(containerPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	provider := fakeProvider{result: Result{Synced: "[00:01.00]hello"}}
	scanner := &fakeScanner{}
	runner := NewRunner(provider, scanner, config.LyricsConfig{
		Enabled:      true,
		FetchWorkers: 1,
		WriteWorkers: 1,
		PathFrom:     "/host/music",
		PathTo:       dir,
		TriggerScan:  true,
	}, log.New(testWriter{t}, "", 0), false)

	stats, err := runner.Run(context.Background(), []model.Track{{
		Title:  "Song",
		Artist: "Artist",
		Album:  "Album",
		Path:   hostPath,
	}})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if stats.Written != 1 {
		t.Fatalf("expected 1 write, got %+v", stats)
	}
	if !scanner.called {
		t.Fatalf("expected scanner call")
	}

	output, err := os.ReadFile(filepath.Join(dir, "Artist", "Album", "song.lrc"))
	if err != nil {
		t.Fatalf("read sidecar: %v", err)
	}
	if string(output) != "[00:01.00]hello\n" {
		t.Fatalf("unexpected sidecar content %q", string(output))
	}
}

func TestRunnerSkipsExistingSidecarUnlessOverwrite(t *testing.T) {
	dir := t.TempDir()
	audioPath := filepath.Join(dir, "song.flac")
	sidecarPath := filepath.Join(dir, "song.lrc")
	if err := os.WriteFile(sidecarPath, []byte("old\n"), 0o644); err != nil {
		t.Fatalf("write existing: %v", err)
	}

	provider := fakeProvider{result: Result{Synced: "[00:01.00]new"}}
	runner := NewRunner(provider, nil, config.LyricsConfig{
		Enabled:      true,
		FetchWorkers: 1,
		WriteWorkers: 1,
	}, log.New(testWriter{t}, "", 0), false)

	stats, err := runner.Run(context.Background(), []model.Track{{Title: "Song", Artist: "Artist", Path: audioPath}})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if stats.Written != 0 || stats.Skipped != 1 {
		t.Fatalf("expected skip, got %+v", stats)
	}

	output, err := os.ReadFile(sidecarPath)
	if err != nil {
		t.Fatalf("read sidecar: %v", err)
	}
	if string(output) != "old\n" {
		t.Fatalf("expected existing content, got %q", string(output))
	}

	runner = NewRunner(provider, nil, config.LyricsConfig{
		Enabled:      true,
		Overwrite:    true,
		FetchWorkers: 1,
		WriteWorkers: 1,
	}, log.New(testWriter{t}, "", 0), false)

	stats, err = runner.Run(context.Background(), []model.Track{{Title: "Song", Artist: "Artist", Path: audioPath}})
	if err != nil {
		t.Fatalf("run overwrite: %v", err)
	}
	if stats.Written != 1 {
		t.Fatalf("expected overwrite write, got %+v", stats)
	}
}

func TestRunnerOverwriteRemovesAlternateSidecar(t *testing.T) {
	dir := t.TempDir()
	audioPath := filepath.Join(dir, "song.flac")
	oldSyncedPath := filepath.Join(dir, "song.lrc")
	if err := os.WriteFile(oldSyncedPath, []byte("old\n"), 0o644); err != nil {
		t.Fatalf("write existing: %v", err)
	}

	runner := NewRunner(fakeProvider{result: Result{Plain: "new plain"}}, nil, config.LyricsConfig{
		Enabled:      true,
		Overwrite:    true,
		FetchWorkers: 1,
		WriteWorkers: 1,
	}, log.New(testWriter{t}, "", 0), false)

	stats, err := runner.Run(context.Background(), []model.Track{{Title: "Song", Artist: "Artist", Path: audioPath}})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if stats.Written != 1 {
		t.Fatalf("expected write, got %+v", stats)
	}
	if _, err := os.Stat(oldSyncedPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected old synced sidecar removal, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "song.txt")); err != nil {
		t.Fatalf("expected plain sidecar: %v", err)
	}
}

type fakeProvider struct {
	result Result
	err    error
}

func (p fakeProvider) Find(context.Context, model.Track) (Result, error) {
	if p.err != nil {
		return Result{}, p.err
	}

	return p.result, nil
}

type fakeScanner struct {
	called bool
}

func (s *fakeScanner) StartScan(context.Context) error {
	s.called = true
	return nil
}

func TestRunnerSkipsProviderNotFound(t *testing.T) {
	runner := NewRunner(fakeProvider{err: ErrNotFound}, nil, config.LyricsConfig{
		Enabled:      true,
		FetchWorkers: 1,
		WriteWorkers: 1,
	}, log.New(testWriter{t}, "", 0), false)

	stats, err := runner.Run(context.Background(), []model.Track{{Title: "Song", Artist: "Artist", Path: "/tmp/song.flac"}})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if stats.Skipped != 1 || stats.Failed != 0 {
		t.Fatalf("expected not found skip, got %+v", stats)
	}
}

func TestRunnerReturnsProviderError(t *testing.T) {
	expected := errors.New("provider down")
	runner := NewRunner(fakeProvider{err: expected}, nil, config.LyricsConfig{
		Enabled:      true,
		FetchWorkers: 1,
		WriteWorkers: 1,
	}, log.New(testWriter{t}, "", 0), false)

	stats, err := runner.Run(context.Background(), []model.Track{{Title: "Song", Artist: "Artist", Path: "/tmp/song.flac"}})
	if err == nil {
		t.Fatalf("expected error")
	}
	if stats.Failed != 1 {
		t.Fatalf("expected failure count, got %+v", stats)
	}
}

type testWriter struct {
	t *testing.T
}

func (w testWriter) Write(p []byte) (int, error) {
	w.t.Logf("%s", p)
	return len(p), nil
}
