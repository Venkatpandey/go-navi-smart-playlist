package state

import (
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	store := NewStore(path, true, log.New(testWriter{t}, "", 0))

	input := NewHistoryState()
	input.UpdatedAt = time.Date(2026, 4, 6, 12, 0, 0, 0, time.UTC)
	input.Tracks["track-1"] = TrackSnapshot{ID: "track-1", PlayCount: 4, SeenCount: 2}
	input.Playlists["Mix"] = PlaylistSnapshot{TrackIDs: []string{"track-1"}}

	if err := store.Save(input); err != nil {
		t.Fatalf("save: %v", err)
	}

	output, err := store.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if output.Tracks["track-1"].PlayCount != 4 {
		t.Fatalf("expected play count 4, got %d", output.Tracks["track-1"].PlayCount)
	}
	if !output.PlaylistContains("Mix", "track-1") {
		t.Fatalf("expected playlist membership to round-trip")
	}
}

func TestLoadCorruptStateFallsBackToEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte("{bad json"), 0o644); err != nil {
		t.Fatalf("write corrupt state: %v", err)
	}

	store := NewStore(path, true, log.New(testWriter{t}, "", 0))
	output, err := store.Load()
	if err == nil {
		t.Fatalf("expected decode error")
	}
	if len(output.Tracks) != 0 {
		t.Fatalf("expected empty state on corrupt load, got %d tracks", len(output.Tracks))
	}
}

type testWriter struct {
	t *testing.T
}

func (w testWriter) Write(p []byte) (int, error) {
	w.t.Logf("%s", p)
	return len(p), nil
}
