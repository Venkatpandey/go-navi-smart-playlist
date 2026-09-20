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
		{ID: "1", Title: "One", Artist: "Artist A", Album: "Album A", Genre: "Rock", Duration: 210, PlayCount: 6, LastPlayed: now.Add(-50 * 24 * time.Hour), Created: now.Add(-200 * 24 * time.Hour), Rating: 4},
		{ID: "2", Title: "Two", Artist: "Artist B", Album: "Album B", Genre: "Jazz", Duration: 600, PlayCount: 4, LastPlayed: now.Add(-90 * 24 * time.Hour), Created: now.Add(-120 * 24 * time.Hour), Rating: 5, Starred: true},
		{ID: "3", Title: "Three", Artist: "Artist C", Album: "Album C", Genre: "Rock", Duration: 190, PlayCount: 0, Created: now.Add(-5 * 24 * time.Hour)},
		{ID: "4", Title: "Four", Artist: "Artist D", Album: "Album D", Genre: "Pop", Duration: 250, PlayCount: 9, LastPlayed: now.Add(-14 * 24 * time.Hour), Created: now.Add(-400 * 24 * time.Hour)},
		{ID: "5", Title: "Five", Artist: "Artist E", Album: "Album E", Genre: "Jazz", Duration: 620, PlayCount: 8, LastPlayed: now.Add(-150 * 24 * time.Hour), Created: now.Add(-500 * 24 * time.Hour)},
		{ID: "6", Title: "Six", Artist: "Artist F", Album: "Album F", Genre: "Rock", Duration: 200, PlayCount: 1, LastPlayed: now.Add(-200 * 24 * time.Hour), Created: now.Add(-220 * 24 * time.Hour)},
		{ID: "7", Title: "Seven", Artist: "Artist G", Album: "Album G", Genre: "Pop", Duration: 300, PlayCount: 5, LastPlayed: now.Add(-20 * 24 * time.Hour), Created: now.Add(-70 * 24 * time.Hour)},
		{ID: "8", Title: "Eight", Artist: "Artist H", Album: "Album H", Genre: "Jazz", Duration: 700, PlayCount: 2, LastPlayed: now.Add(-300 * 24 * time.Hour), Created: now.Add(-350 * 24 * time.Hour), Starred: true},
	}

	previous := state.NewHistoryState()
	previous.Playlists["Comfort Shuffle"] = state.PlaylistSnapshot{TrackIDs: []string{"1"}}
	previous.Tracks["1"] = state.TrackSnapshot{ID: "1", PlayCount: 5, SeenCount: 3}
	previous.Tracks["2"] = state.TrackSnapshot{ID: "2", PlayCount: 4, SeenCount: 3}
	previous.Tracks["4"] = state.TrackSnapshot{ID: "4", PlayCount: 8, SeenCount: 3}

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
	if len(playlists) != 14 {
		t.Fatalf("expected 14 playlists, got %d", len(playlists))
	}

	required := map[string]int{
		"Rediscover":              1,
		"Long Time No See":        1,
		"Comfort Shuffle":         1,
		"More Like Hidden Gems":   1,
		"Artist Adjacent Comfort": 1,
		"Fresh & Unplayed":        1,
		"Forgotten Favorites":     1,
		"Rising This Week":        1,
		"Deep Cuts":               1,
		"Quick Mix":               1,
		"Longform":                1,
	}

	for _, definition := range playlists {
		if minTracks, ok := required[definition.Name]; ok && len(definition.Tracks) < minTracks {
			t.Fatalf("expected playlist %q to have at least %d track, got %d", definition.Name, minTracks, len(definition.Tracks))
		}
	}

	assertTracksMatch(t, playlists, "Long Time No See", func(track model.Track) bool {
		return !track.LastPlayed.IsZero() && now.Sub(track.LastPlayed) >= 120*24*time.Hour
	})
	assertTracksMatch(t, playlists, "Fresh & Unplayed", func(track model.Track) bool {
		return track.LastPlayed.IsZero() && now.Sub(track.Created) <= 180*24*time.Hour
	})
	assertTracksMatch(t, playlists, "Rising This Week", func(track model.Track) bool {
		return track.ID == "4"
	})
	assertTracksMatch(t, playlists, "Quick Mix", func(track model.Track) bool {
		return track.Duration > 0 && track.Duration <= quickTrackSeconds
	})
	assertTracksMatch(t, playlists, "Longform", func(track model.Track) bool {
		return track.Duration >= longformSeconds
	})
}

func TestSimilarPlaylistExcludesEverySeedTrack(t *testing.T) {
	now := time.Date(2026, 4, 6, 12, 0, 0, 0, time.UTC)
	seeds := make([]features.TrackFeatures, 0, 15)
	items := make([]features.TrackFeatures, 0, 20)
	for index := 0; index < 15; index++ {
		seed := features.TrackFeatures{
			Track:            model.Track{ID: "seed-" + string(rune('a'+index)), Artist: "Seed Artist", Album: "Seed Album"},
			SimilarityVector: []float64{1, 0.5},
		}
		seeds = append(seeds, seed)
		items = append(items, seed)
	}
	for index := 0; index < 5; index++ {
		items = append(items, features.TrackFeatures{
			Track: model.Track{
				ID:     "candidate-" + string(rune('a'+index)),
				Artist: "Candidate Artist " + string(rune('a'+index)),
				Album:  "Candidate Album " + string(rune('a'+index)),
			},
			SimilarityVector: []float64{1, 0.5},
		})
	}

	generator := newTestGenerator(t, 20, 0)
	result := generator.similarPlaylist(
		"Similar",
		features.Dataset{Items: items},
		state.NewHistoryState(),
		now,
		seeds,
		false,
		0,
		0,
		nil,
		func(_ features.TrackFeatures, similarityScore, _ float64) float64 { return similarityScore },
	)

	if len(result) != 5 {
		t.Fatalf("expected five non-seed candidates, got %d", len(result))
	}
	for _, track := range result {
		if len(track.Track.ID) >= 5 && track.Track.ID[:5] == "seed-" {
			t.Fatalf("seed track leaked into similar playlist: %s", track.Track.ID)
		}
	}
}

func TestWeeklyJitterIsStableWithinWeekAndRotatesAcrossWeeks(t *testing.T) {
	first := time.Date(2026, 4, 6, 12, 0, 0, 0, time.UTC)
	if centeredWeeklyJitter("Mix", "track", first) != centeredWeeklyJitter("Mix", "track", first.Add(2*24*time.Hour)) {
		t.Fatalf("jitter changed within same ISO week")
	}
	if centeredWeeklyJitter("Mix", "track", first) == centeredWeeklyJitter("Mix", "track", first.Add(7*24*time.Hour)) {
		t.Fatalf("jitter did not change across ISO weeks")
	}
}

func TestAgeCurvesHaveExpectedDirection(t *testing.T) {
	if freshnessCurve(5, 0, 180) <= freshnessCurve(170, 0, 180) {
		t.Fatalf("newer track must receive stronger freshness score")
	}
	if longTailScore(120, 120, 60) != 0 {
		t.Fatalf("long-tail score must start at zero at threshold")
	}
	if longTailScore(300, 120, 60) <= longTailScore(180, 120, 60) {
		t.Fatalf("older eligible track must receive stronger long-tail score")
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
	generator := newTestGenerator(t, 8, 0)

	playlists := generator.Generate(dataset, state.NewHistoryState(), now)
	for _, definition := range playlists {
		if len(definition.Tracks) > 5 {
			t.Fatalf("expected diversity-limited playlist %q to top out at 5 tracks, got %d", definition.Name, len(definition.Tracks))
		}
	}
}

func newTestGenerator(t *testing.T, size, minBackfill int) *Generator {
	t.Helper()
	return NewGenerator(config.Config{
		PlaylistSize: size,
		MinBackfill:  minBackfill,
		Weights: config.Weights{
			PlayCount: 1,
			Recency:   2,
			Freshness: 1.5,
			DecayDays: 45,
		},
	}, log.New(testWriter{t}, "", 0))
}

func assertTracksMatch(t *testing.T, playlists []Definition, name string, predicate func(model.Track) bool) {
	t.Helper()
	for _, definition := range playlists {
		if definition.Name != name {
			continue
		}
		for _, track := range definition.Tracks {
			if !predicate(track) {
				t.Fatalf("playlist %q contains ineligible track %+v", name, track)
			}
		}
		return
	}
	t.Fatalf("missing playlist %q", name)
}

func TestArtistInterleaving(t *testing.T) {
	input := []model.Track{
		{ID: "1", Artist: "Artist A", Title: "A1"},
		{ID: "2", Artist: "Artist A", Title: "A2"},
		{ID: "3", Artist: "Artist A", Title: "A3"},
		{ID: "4", Artist: "Artist B", Title: "B1"},
		{ID: "5", Artist: "Artist B", Title: "B2"},
		{ID: "6", Artist: "Artist C", Title: "C1"},
		{ID: "7", Artist: "Artist D", Title: "D1"},
	}

	result := interleaveArtists(input)
	if len(result) != len(input) {
		t.Fatalf("expected %d tracks, got %d", len(input), len(result))
	}

	for i := 1; i < len(result); i++ {
		if result[i].Artist == result[i-1].Artist {
			t.Fatalf("consecutive tracks from same artist %q at index %d and %d: %+v", result[i].Artist, i-1, i, result)
		}
	}
}

func TestRecipeExclusivityFreshVsDiscover(t *testing.T) {
	generator := newTestGenerator(t, 10, 0)
	recipes := generator.recipes()
	var freshRecipe, discoverRecipe recipe
	for _, r := range recipes {
		if r.name == "Fresh & Unplayed" {
			freshRecipe = r
		}
		if r.name == "Discover Weekly" {
			discoverRecipe = r
		}
	}

	now := time.Date(2026, 4, 6, 12, 0, 0, 0, time.UTC)

	// Unplayed track added 10 days ago (fresh unplayed)
	freshTrack := features.TrackFeatures{
		Track:          model.Track{ID: "fresh", PlayCount: 0, Created: now.Add(-10 * 24 * time.Hour)},
		DaysSinceAdded: 10,
		HasLastPlayed:  false,
	}
	if !freshRecipe.eligible(freshTrack) {
		t.Fatalf("expected fresh unplayed track to be eligible for Fresh & Unplayed")
	}
	if discoverRecipe.eligible(freshTrack) {
		t.Fatalf("expected fresh unplayed track NOT to be eligible for Discover Weekly")
	}

	// Unplayed track added 200 days ago (older unplayed)
	olderUnplayed := features.TrackFeatures{
		Track:          model.Track{ID: "older", PlayCount: 0, Created: now.Add(-200 * 24 * time.Hour)},
		DaysSinceAdded: 200,
		HasLastPlayed:  false,
	}
	if freshRecipe.eligible(olderUnplayed) {
		t.Fatalf("expected older unplayed track NOT to be eligible for Fresh & Unplayed")
	}
	if !discoverRecipe.eligible(olderUnplayed) {
		t.Fatalf("expected older unplayed track to be eligible for Discover Weekly")
	}

	// Low play track (played 3 times)
	lowPlayTrack := features.TrackFeatures{
		Track:         model.Track{ID: "lowplay", PlayCount: 3, LastPlayed: now.Add(-30 * 24 * time.Hour)},
		HasLastPlayed: true,
	}
	if freshRecipe.eligible(lowPlayTrack) {
		t.Fatalf("expected played track NOT to be eligible for Fresh & Unplayed")
	}
	if !discoverRecipe.eligible(lowPlayTrack) {
		t.Fatalf("expected low play track to be eligible for Discover Weekly")
	}
}

func TestMultiRunCooldownDecay(t *testing.T) {
	generator := newTestGenerator(t, 10, 0)
	now := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)

	stateRecent := state.NewHistoryState()
	stateRecent.Tracks["t1"] = state.TrackSnapshot{
		ID:             "t1",
		LastFeaturedAt: now.Add(-3 * 24 * time.Hour), // 3 days ago
		LastFeaturedIn: map[string]time.Time{"Discover Weekly": now.Add(-3 * 24 * time.Hour)},
	}

	stateWeek2 := state.NewHistoryState()
	stateWeek2.Tracks["t1"] = state.TrackSnapshot{
		ID:             "t1",
		LastFeaturedAt: now.Add(-10 * 24 * time.Hour), // 10 days ago (week 2)
		LastFeaturedIn: map[string]time.Time{"Discover Weekly": now.Add(-10 * 24 * time.Hour)},
	}

	stateWeek3 := state.NewHistoryState()
	stateWeek3.Tracks["t1"] = state.TrackSnapshot{
		ID:             "t1",
		LastFeaturedAt: now.Add(-18 * 24 * time.Hour), // 18 days ago (week 3)
		LastFeaturedIn: map[string]time.Time{"Discover Weekly": now.Add(-18 * 24 * time.Hour)},
	}

	stateOld := state.NewHistoryState()
	stateOld.Tracks["t1"] = state.TrackSnapshot{
		ID:             "t1",
		LastFeaturedAt: now.Add(-30 * 24 * time.Hour), // 30 days ago
		LastFeaturedIn: map[string]time.Time{"Discover Weekly": now.Add(-30 * 24 * time.Hour)},
	}

	penRecent := generator.playlistCarryover(stateRecent, "Discover Weekly", "t1", -0.65, now)
	penWeek2 := generator.playlistCarryover(stateWeek2, "Discover Weekly", "t1", -0.65, now)
	penWeek3 := generator.playlistCarryover(stateWeek3, "Discover Weekly", "t1", -0.65, now)
	penOld := generator.playlistCarryover(stateOld, "Discover Weekly", "t1", -0.65, now)

	// Verify decaying penalty: recent is most negative, then week 2, then week 3, then old has no penalty
	if penRecent >= penWeek2 || penWeek2 >= penWeek3 || penWeek3 >= penOld {
		t.Fatalf("expected strictly decaying penalties: recent=%.3f, week2=%.3f, week3=%.3f, old=%.3f",
			penRecent, penWeek2, penWeek3, penOld)
	}
	if penOld != 0 {
		t.Fatalf("expected old state to have 0 penalty, got %.3f", penOld)
	}
}

type testWriter struct {
	t *testing.T
}

func (w testWriter) Write(p []byte) (int, error) {
	w.t.Logf("%s", p)
	return len(p), nil
}
