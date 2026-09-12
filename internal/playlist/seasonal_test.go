package playlist

import (
	"fmt"
	"testing"
	"time"

	"go-navi-smart-playlist/internal/features"
	"go-navi-smart-playlist/internal/model"
	"go-navi-smart-playlist/internal/state"
)

func TestSeasonalUpdateStartsWithBaseline(t *testing.T) {
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	dataset := features.Dataset{Items: []features.TrackFeatures{{
		Track:          model.Track{ID: "old-favorite", PlayCount: 100, LastPlayed: now},
		PlayCountDelta: 100,
	}}}

	update := newTestGenerator(t, 20, 0).UpdateSeasonal(dataset, state.NewHistoryState(), now)

	if update.Definition != nil {
		t.Fatalf("did not expect a recap during the current quarter")
	}
	if update.Snapshot.Period != "2026-Q3" {
		t.Fatalf("expected summer period, got %q", update.Snapshot.Period)
	}
	if len(update.Snapshot.PlayCounts) != 0 {
		t.Fatalf("lifetime play counts must not be recorded on first run: %+v", update.Snapshot.PlayCounts)
	}
}

func TestSeasonalUpdateAccumulatesOnlyCurrentQuarter(t *testing.T) {
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	previous := state.NewHistoryState()
	previous.Seasonal = state.SeasonalSnapshot{
		Period:     "2026-Q3",
		PlayCounts: map[string]int{"summer": 2},
	}
	dataset := features.Dataset{Items: []features.TrackFeatures{
		{Track: model.Track{ID: "summer", LastPlayed: now}, PlayCountDelta: 3},
		{Track: model.Track{ID: "spring", LastPlayed: time.Date(2026, 6, 30, 23, 0, 0, 0, time.UTC)}, PlayCountDelta: 4},
	}}

	update := newTestGenerator(t, 20, 0).UpdateSeasonal(dataset, previous, now)

	if update.Definition != nil {
		t.Fatalf("did not expect a recap during the current quarter")
	}
	if update.Snapshot.PlayCounts["summer"] != 5 {
		t.Fatalf("expected five summer plays, got %+v", update.Snapshot.PlayCounts)
	}
	if _, found := update.Snapshot.PlayCounts["spring"]; found {
		t.Fatalf("spring plays leaked into summer: %+v", update.Snapshot.PlayCounts)
	}
}

func TestSeasonalUpdateCreatesCompletedRecapAndStartsNewQuarter(t *testing.T) {
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	previous := state.NewHistoryState()
	previous.Seasonal = state.SeasonalSnapshot{
		Period: "2026-Q3",
		PlayCounts: map[string]int{
			"first":  5,
			"second": 4,
		},
	}
	dataset := features.Dataset{Items: []features.TrackFeatures{
		{Track: model.Track{ID: "first", PlayCount: 20, LastPlayed: time.Date(2026, 9, 30, 22, 0, 0, 0, time.UTC)}, PlayCountDelta: 2},
		{Track: model.Track{ID: "second", PlayCount: 30, LastPlayed: time.Date(2026, 9, 29, 22, 0, 0, 0, time.UTC)}},
		{Track: model.Track{ID: "new-quarter", PlayCount: 1, LastPlayed: now}, PlayCountDelta: 1},
	}}

	update := newTestGenerator(t, 20, 0).UpdateSeasonal(dataset, previous, now)

	if update.Definition == nil {
		t.Fatalf("expected completed summer recap")
	}
	if update.Definition.Name != "your summer 2026 recap" {
		t.Fatalf("unexpected recap name %q", update.Definition.Name)
	}
	if got := trackIDs(update.Definition.Tracks); fmt.Sprint(got) != "[first second]" {
		t.Fatalf("expected tracks ordered by quarter plays, got %v", got)
	}
	if update.Snapshot.Period != "2026-Q4" || update.Snapshot.PlayCounts["new-quarter"] != 1 {
		t.Fatalf("expected fresh autumn activity, got %+v", update.Snapshot)
	}
}

func TestSeasonalDefinitionLimitsRecapToTopFifty(t *testing.T) {
	snapshot := state.SeasonalSnapshot{Period: "2026-Q1", PlayCounts: map[string]int{}}
	items := make([]features.TrackFeatures, 0, 55)
	for index := 0; index < 55; index++ {
		id := fmt.Sprintf("track-%02d", index)
		snapshot.PlayCounts[id] = index + 1
		items = append(items, features.TrackFeatures{Track: model.Track{ID: id}})
	}

	definition := seasonalDefinition(snapshot, items)
	if definition == nil || len(definition.Tracks) != seasonalTrackLimit {
		t.Fatalf("expected %d tracks, got %+v", seasonalTrackLimit, definition)
	}
	if definition.Tracks[0].ID != "track-54" || definition.Tracks[49].ID != "track-05" {
		t.Fatalf("unexpected top-fifty bounds: first=%q last=%q", definition.Tracks[0].ID, definition.Tracks[49].ID)
	}
}

func trackIDs(tracks []model.Track) []string {
	ids := make([]string, 0, len(tracks))
	for _, track := range tracks {
		ids = append(ids, track.ID)
	}
	return ids
}
