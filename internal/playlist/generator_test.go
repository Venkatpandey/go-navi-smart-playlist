package playlist

import (
	"log"
	"testing"
	"time"

	"go-navi-smart-playlist/internal/config"
	"go-navi-smart-playlist/internal/features"
	"go-navi-smart-playlist/internal/model"
	"go-navi-smart-playlist/internal/state"
)

func TestGeneratorProducesNonEmptySoftRankedPlaylists(t *testing.T) {
	now := time.Date(2026, 4, 6, 12, 0, 0, 0, time.UTC)
	tracks := []model.Track{
		{ID: "1", Title: "One", Artist: "Artist A", Album: "Album A", PlayCount: 6, LastPlayed: now.Add(-50 * 24 * time.Hour), Created: now.Add(-200 * 24 * time.Hour), Rating: 4},
		{ID: "2", Title: "Two", Artist: "Artist B", Album: "Album B", PlayCount: 4, LastPlayed: now.Add(-90 * 24 * time.Hour), Created: now.Add(-120 * 24 * time.Hour), Rating: 5, Starred: true},
		{ID: "3", Title: "Three", Artist: "Artist C", Album: "Album C", PlayCount: 0, Created: now.Add(-5 * 24 * time.Hour)},
		{ID: "4", Title: "Four", Artist: "Artist D", Album: "Album D", PlayCount: 9, LastPlayed: now.Add(-14 * 24 * time.Hour), Created: now.Add(-400 * 24 * time.Hour)},
		{ID: "5", Title: "Five", Artist: "Artist E", Album: "Album E", PlayCount: 8, LastPlayed: now.Add(-150 * 24 * time.Hour), Created: now.Add(-500 * 24 * time.Hour)},
		{ID: "6", Title: "Six", Artist: "Artist F", Album: "Album F", PlayCount: 1, LastPlayed: now.Add(-200 * 24 * time.Hour), Created: now.Add(-220 * 24 * time.Hour)},
		{ID: "7", Title: "Seven", Artist: "Artist G", Album: "Album G", PlayCount: 5, LastPlayed: now.Add(-20 * 24 * time.Hour), Created: now.Add(-70 * 24 * time.Hour)},
		{ID: "8", Title: "Eight", Artist: "Artist H", Album: "Album H", PlayCount: 2, LastPlayed: now.Add(-300 * 24 * time.Hour), Created: now.Add(-350 * 24 * time.Hour)},
	}

	previous := state.NewHistoryState()
	previous.Playlists["Comfort Shuffle"] = state.PlaylistSnapshot{TrackIDs: []string{"1"}}
	previous.Tracks["1"] = state.TrackSnapshot{ID: "1", PlayCount: 5, SeenCount: 3}
	previous.Tracks["2"] = state.TrackSnapshot{ID: "2", PlayCount: 4, SeenCount: 3}

	builder := features.NewBuilder(log.New(testWriter{t}, "", 0))
	dataset := builder.Build(tracks, previous, now)
	generator := NewGenerator(config.Config{
		PlaylistSize: 5,
		MinBackfill:  3,
		Weights: config.Weights{
			PlayCount: 1,
			Recency:   2,
			Freshness: 1.5,
			DecayDays: 45,
		},
	}, log.New(testWriter{t}, "", 0))

	playlists := generator.Generate(dataset, previous, now)
	if len(playlists) < 8 {
		t.Fatalf("expected 8 playlists, got %d", len(playlists))
	}

	required := map[string]int{
		"Rediscover":              1,
		"Long Time No See":        1,
		"Comfort Shuffle":         1,
		"More Like Hidden Gems":   1,
		"Artist Adjacent Comfort": 1,
	}

	for _, definition := range playlists {
		if minTracks, ok := required[definition.Name]; ok && len(definition.Tracks) < minTracks {
			t.Fatalf("expected playlist %q to have at least %d track, got %d", definition.Name, minTracks, len(definition.Tracks))
		}
	}
}

func TestGeneratorEnforcesDiversityCaps(t *testing.T) {
	now := time.Date(2026, 4, 6, 12, 0, 0, 0, time.UTC)
	var tracks []model.Track
	for index := 0; index < 8; index++ {
		tracks = append(tracks, model.Track{
			ID:         string(rune('a' + index)),
			Title:      "Track",
			Artist:     "Artist A",
			Album:      "Album A",
			PlayCount:  10 - index,
			LastPlayed: now.Add(-time.Duration(10+index) * 24 * time.Hour),
			Created:    now.Add(-300 * 24 * time.Hour),
		})
	}

	builder := features.NewBuilder(log.New(testWriter{t}, "", 0))
	dataset := builder.Build(tracks, state.NewHistoryState(), now)
	generator := NewGenerator(config.Config{
		PlaylistSize: 8,
		MinBackfill:  0,
		Weights: config.Weights{
			PlayCount: 1,
			Recency:   2,
			Freshness: 1.5,
			DecayDays: 45,
		},
	}, log.New(testWriter{t}, "", 0))

	playlists := generator.Generate(dataset, state.NewHistoryState(), now)
	for _, definition := range playlists {
		if len(definition.Tracks) > 5 {
			t.Fatalf("expected diversity-limited playlist %q to top out at 5 tracks, got %d", definition.Name, len(definition.Tracks))
		}
	}
}

type testWriter struct {
	t *testing.T
}

func (w testWriter) Write(p []byte) (int, error) {
	w.t.Logf("%s", p)
	return len(p), nil
}
