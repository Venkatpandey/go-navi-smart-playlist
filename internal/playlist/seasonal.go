package playlist

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"go-navi-smart-playlist/internal/features"
	"go-navi-smart-playlist/internal/model"
	"go-navi-smart-playlist/internal/state"
)

const seasonalTrackLimit = 50

type SeasonalUpdate struct {
	Definition *Definition
	Snapshot   state.SeasonalSnapshot
}

type seasonalPeriod struct {
	key    string
	season string
	year   int
}

type seasonalTrack struct {
	track model.Track
	plays int
}

// UpdateSeasonal records play-count increases for the current calendar quarter.
// On the first run in a new quarter, it returns an immutable recap for the
// completed quarter and starts a fresh activity snapshot.
func (g *Generator) UpdateSeasonal(
	dataset features.Dataset,
	previous *state.HistoryState,
	now time.Time,
) SeasonalUpdate {
	current := seasonalPeriodAt(now)
	currentSnapshot := newSeasonalSnapshot(current.key)
	if previous == nil {
		return SeasonalUpdate{Snapshot: currentSnapshot}
	}

	priorSnapshot := cloneSeasonalSnapshot(previous.Seasonal)
	if priorSnapshot.Period == "" {
		// Existing version-one state provides a usable play-count baseline. A
		// brand-new state must not mistake lifetime play counts for this quarter.
		if !previous.UpdatedAt.IsZero() {
			addSeasonalDeltas(currentSnapshot.PlayCounts, dataset.Items, current.key)
		}
		return SeasonalUpdate{Snapshot: currentSnapshot}
	}

	if priorSnapshot.Period == current.key {
		addSeasonalDeltas(priorSnapshot.PlayCounts, dataset.Items, current.key)
		return SeasonalUpdate{Snapshot: priorSnapshot}
	}

	addSeasonalDeltas(priorSnapshot.PlayCounts, dataset.Items, priorSnapshot.Period)
	addSeasonalDeltas(currentSnapshot.PlayCounts, dataset.Items, current.key)

	definition := seasonalDefinition(priorSnapshot, dataset.Items)
	if definition == nil {
		g.logger.Printf("seasonal recap for %q has no available played tracks, skipping create", priorSnapshot.Period)
	} else {
		g.logger.Printf("generated seasonal playlist %q with %d tracks", definition.Name, len(definition.Tracks))
	}

	return SeasonalUpdate{
		Definition: definition,
		Snapshot:   currentSnapshot,
	}
}

func seasonalPeriodAt(when time.Time) seasonalPeriod {
	year := when.Year()
	quarter := (int(when.Month())-1)/3 + 1
	seasons := [...]string{"winter", "spring", "summer", "autumn"}
	return seasonalPeriod{
		key:    fmt.Sprintf("%04d-Q%d", year, quarter),
		season: seasons[quarter-1],
		year:   year,
	}
}

func parseSeasonalPeriod(key string) (seasonalPeriod, bool) {
	parts := strings.Split(key, "-Q")
	if len(parts) != 2 {
		return seasonalPeriod{}, false
	}

	year, yearErr := strconv.Atoi(parts[0])
	quarter, quarterErr := strconv.Atoi(parts[1])
	if yearErr != nil || quarterErr != nil || quarter < 1 || quarter > 4 {
		return seasonalPeriod{}, false
	}

	seasons := [...]string{"winter", "spring", "summer", "autumn"}
	return seasonalPeriod{key: key, season: seasons[quarter-1], year: year}, true
}

func newSeasonalSnapshot(period string) state.SeasonalSnapshot {
	return state.SeasonalSnapshot{
		Period:     period,
		PlayCounts: map[string]int{},
	}
}

func cloneSeasonalSnapshot(snapshot state.SeasonalSnapshot) state.SeasonalSnapshot {
	result := newSeasonalSnapshot(snapshot.Period)
	for trackID, plays := range snapshot.PlayCounts {
		if plays > 0 {
			result.PlayCounts[trackID] = plays
		}
	}
	return result
}

func addSeasonalDeltas(counts map[string]int, items []features.TrackFeatures, period string) {
	for _, item := range items {
		delta := int(item.PlayCountDelta)
		if delta <= 0 || item.Track.LastPlayed.IsZero() || seasonalPeriodAt(item.Track.LastPlayed).key != period {
			continue
		}
		counts[item.Track.ID] += delta
	}
}

func seasonalDefinition(snapshot state.SeasonalSnapshot, items []features.TrackFeatures) *Definition {
	period, ok := parseSeasonalPeriod(snapshot.Period)
	if !ok {
		return nil
	}

	available := make(map[string]model.Track, len(items))
	for _, item := range items {
		available[item.Track.ID] = item.Track
	}

	ranked := make([]seasonalTrack, 0, len(snapshot.PlayCounts))
	for trackID, plays := range snapshot.PlayCounts {
		track, found := available[trackID]
		if !found || plays <= 0 {
			continue
		}
		ranked = append(ranked, seasonalTrack{track: track, plays: plays})
	}

	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].plays != ranked[j].plays {
			return ranked[i].plays > ranked[j].plays
		}
		if ranked[i].track.PlayCount != ranked[j].track.PlayCount {
			return ranked[i].track.PlayCount > ranked[j].track.PlayCount
		}
		return ranked[i].track.ID < ranked[j].track.ID
	})

	if len(ranked) > seasonalTrackLimit {
		ranked = ranked[:seasonalTrackLimit]
	}
	if len(ranked) == 0 {
		return nil
	}

	tracks := make([]model.Track, 0, len(ranked))
	for _, item := range ranked {
		tracks = append(tracks, item.track)
	}

	return &Definition{
		Name:   fmt.Sprintf("your %s %d recap", period.season, period.year),
		Tracks: tracks,
	}
}
