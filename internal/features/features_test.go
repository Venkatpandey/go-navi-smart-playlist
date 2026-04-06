package features

import (
	"log"
	"testing"
	"time"

	"go-navi-smart-playlist/internal/model"
	"go-navi-smart-playlist/internal/state"
)

func TestBuildDerivesExpectedSignals(t *testing.T) {
	now := time.Date(2026, 4, 6, 12, 0, 0, 0, time.UTC)
	tracks := []model.Track{
		{
			ID:         "rising",
			Title:      "Rising Song",
			Artist:     "Artist A",
			Album:      "Album A",
			PlayCount:  6,
			LastPlayed: now.Add(-2 * 24 * time.Hour),
			Created:    now.Add(-10 * 24 * time.Hour),
			Rating:     4,
			Starred:    true,
		},
		{
			ID:        "new",
			Title:     "New Song",
			Artist:    "Artist B",
			Album:     "Album B",
			PlayCount: 0,
			Created:   now.Add(-3 * 24 * time.Hour),
		},
	}

	previous := state.NewHistoryState()
	previous.Tracks["rising"] = state.TrackSnapshot{
		ID:        "rising",
		PlayCount: 3,
		SeenCount: 2,
	}

	builder := NewBuilder(log.New(testWriter{t}, "", 0))
	dataset := builder.Build(tracks, previous, now)

	if dataset.Stats.TotalTracks != 2 {
		t.Fatalf("expected 2 tracks, got %d", dataset.Stats.TotalTracks)
	}
	if dataset.Stats.TracksWithLastPlayed != 1 {
		t.Fatalf("expected 1 track with last played, got %d", dataset.Stats.TracksWithLastPlayed)
	}

	rising, ok := dataset.Get("rising")
	if !ok {
		t.Fatalf("missing rising track features")
	}
	if rising.PlayCountDelta != 3 {
		t.Fatalf("expected play delta 3, got %.2f", rising.PlayCountDelta)
	}
	if rising.RecencyTrendScore <= 0 {
		t.Fatalf("expected positive recency trend, got %.4f", rising.RecencyTrendScore)
	}
	if rising.StabilityScore <= 0 {
		t.Fatalf("expected positive stability score, got %.4f", rising.StabilityScore)
	}

	newTrack, ok := dataset.Get("new")
	if !ok {
		t.Fatalf("missing new track features")
	}
	if newTrack.HasLastPlayed {
		t.Fatalf("expected new track to have no last played value")
	}
	if newTrack.NoveltyScore <= rising.NoveltyScore {
		t.Fatalf("expected new track novelty %.4f to exceed rising track novelty %.4f", newTrack.NoveltyScore, rising.NoveltyScore)
	}
}

type testWriter struct {
	t *testing.T
}

func (w testWriter) Write(p []byte) (int, error) {
	w.t.Logf("%s", p)
	return len(p), nil
}
