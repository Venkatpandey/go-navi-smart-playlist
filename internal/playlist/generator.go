package playlist

import (
	"fmt"
	"hash/fnv"
	"log"
	"math"
	"sort"
	"strings"
	"time"

	"go-navi-smart-playlist/internal/config"
	"go-navi-smart-playlist/internal/features"
	"go-navi-smart-playlist/internal/model"
	"go-navi-smart-playlist/internal/scoring"
	"go-navi-smart-playlist/internal/similarity"
	"go-navi-smart-playlist/internal/state"
)

const (
	defaultArtistLimit = 3
	defaultAlbumLimit  = 3
	maxBackfillLimit   = 5
	quickTrackSeconds  = 4 * 60
	longformSeconds    = 8 * 60
)

type Definition struct {
	Name   string
	Tracks []model.Track
}

type Generator struct {
	size        int
	minBackfill int
	engine      *scoring.Engine
	logger      *log.Logger
}

type scoredTrack struct {
	track features.TrackFeatures
	score float64
}

type recipe struct {
	name       string
	eligible   func(features.TrackFeatures) bool
	score      func(features.TrackFeatures) float64
	carryover  float64
	weekJitter float64
}

func NewGenerator(cfg config.Config, logger *log.Logger) *Generator {
	return &Generator{
		size:        cfg.PlaylistSize,
		minBackfill: cfg.MinBackfill,
		engine:      scoring.New(cfg.Weights),
		logger:      logger,
	}
}

func (g *Generator) Generate(dataset features.Dataset, previous *state.HistoryState, now time.Time) []Definition {
	ranked := make(map[string][]features.TrackFeatures)
	for _, currentRecipe := range g.recipes() {
		ranked[currentRecipe.name] = g.rankRecipe(currentRecipe, dataset.Items, previous, now)
	}

	ranked["More Like Hidden Gems"] = g.similarPlaylist(
		"More Like Hidden Gems",
		dataset,
		previous,
		now,
		ranked["Hidden Gems"],
		false,
		-0.35,
		0.25,
		func(track features.TrackFeatures, similarityScore, genreMatch float64) float64 {
			return 1.6*similarityScore + 0.3*genreMatch + g.engine.BaseScore(track) + 0.8*track.NoveltyScore
		},
	)

	ranked["Artist Adjacent Comfort"] = g.similarPlaylist(
		"Artist Adjacent Comfort",
		dataset,
		previous,
		now,
		ranked["Comfort Shuffle"],
		true,
		-0.2,
		0.35,
		func(track features.TrackFeatures, similarityScore, genreMatch float64) float64 {
			return 1.5*similarityScore + 0.4*genreMatch + g.engine.BaseScore(track) + 0.5*track.StabilityScore
		},
	)

	names := []string{
		"Discover Weekly",
		"Rediscover",
		"Top This Month",
		"Hidden Gems",
		"Long Time No See",
		"Comfort Shuffle",
		"More Like Hidden Gems",
		"Artist Adjacent Comfort",
		"Fresh & Unplayed",
		"Forgotten Favorites",
		"Rising This Week",
		"Deep Cuts",
		"Quick Mix",
		"Longform",
	}

	definitions := make([]Definition, 0, len(names))
	for _, name := range names {
		definition := Definition{Name: name, Tracks: toTracks(ranked[name])}
		definitions = append(definitions, definition)
		g.logger.Printf("generated playlist %q with %d tracks", definition.Name, len(definition.Tracks))
	}

	return definitions
}

func (g *Generator) recipes() []recipe {
	return []recipe{
		{
			name: "Discover Weekly",
			eligible: func(track features.TrackFeatures) bool {
				return !track.HasLastPlayed || track.Track.PlayCount <= 3 || track.DaysSinceAdded <= 180
			},
			score: func(track features.TrackFeatures) float64 {
				return g.engine.BaseScore(track) +
					1.6*track.NoveltyScore +
					0.6*freshnessCurve(track.DaysSinceAdded, 0, 180) -
					0.7*track.PlayCountPercentile -
					0.4*track.RepeatFatigueScore
			},
			carryover:  -0.65,
			weekJitter: 0.35,
		},
		{
			name: "Rediscover",
			eligible: func(track features.TrackFeatures) bool {
				return track.HasLastPlayed && track.DaysSinceLastPlayed >= 45 && track.DaysSinceLastPlayed <= 720
			},
			score: func(track features.TrackFeatures) float64 {
				return g.engine.BaseScore(track) +
					1.4*windowScore(track.DaysSinceLastPlayed, 60, 365, 45) +
					0.9*track.PlayCountPercentile +
					0.4*track.StabilityScore -
					0.3*track.RepeatFatigueScore
			},
			carryover:  -0.1,
			weekJitter: 0.15,
		},
		{
			name: "Top This Month",
			eligible: func(track features.TrackFeatures) bool {
				return track.HasLastPlayed && track.DaysSinceLastPlayed <= 31 && (track.PlayCountDelta > 0 || track.Track.PlayCount > 0)
			},
			score: func(track features.TrackFeatures) float64 {
				return g.engine.BaseScore(track) +
					1.8*windowScore(track.DaysSinceLastPlayed, 0, 31, 12) +
					1.2*deltaScore(track.PlayCountDelta) +
					0.5*track.PlayCountPercentile
			},
			carryover:  0.15,
			weekJitter: 0.05,
		},
		{
			name: "Hidden Gems",
			eligible: func(track features.TrackFeatures) bool {
				return track.PlayCountPercentile <= 0.55 || track.Track.Starred || track.Track.Rating >= 4
			},
			score: func(track features.TrackFeatures) float64 {
				return g.engine.BaseScore(track) +
					1.5*track.NoveltyScore +
					0.8*(1-track.PlayCountPercentile) +
					0.45*preferenceScore(track) +
					0.3*windowScore(track.DaysSinceLastPlayed, 21, 240, 60)
			},
			carryover:  -0.3,
			weekJitter: 0.25,
		},
		{
			name: "Long Time No See",
			eligible: func(track features.TrackFeatures) bool {
				return track.HasLastPlayed && track.DaysSinceLastPlayed >= 120
			},
			score: func(track features.TrackFeatures) float64 {
				return g.engine.BaseScore(track) +
					1.6*longTailScore(track.DaysSinceLastPlayed, 120, 60) +
					0.8*track.PlayCountPercentile +
					0.4*track.StabilityScore
			},
			carryover:  -0.4,
			weekJitter: 0.2,
		},
		{
			name: "Comfort Shuffle",
			eligible: func(track features.TrackFeatures) bool {
				return track.HasLastPlayed && (track.PlayCountPercentile >= 0.55 || track.Track.Starred || track.Track.Rating >= 4)
			},
			score: func(track features.TrackFeatures) float64 {
				return g.engine.BaseScore(track) +
					1.1*preferenceScore(track) +
					0.5*track.StabilityScore -
					0.7*track.RepeatFatigueScore
			},
			carryover:  0.1,
			weekJitter: 0.9,
		},
		{
			name: "Fresh & Unplayed",
			eligible: func(track features.TrackFeatures) bool {
				return !track.HasLastPlayed && track.DaysSinceAdded <= 180
			},
			score: func(track features.TrackFeatures) float64 {
				return 2.0*freshnessCurve(track.DaysSinceAdded, 0, 180) +
					1.8*track.NoveltyScore +
					0.4*track.ArtistAffinity +
					0.3*preferenceScore(track)
			},
			carryover:  -0.8,
			weekJitter: 0.4,
		},
		{
			name: "Forgotten Favorites",
			eligible: func(track features.TrackFeatures) bool {
				favorite := track.Track.Starred || track.Track.Rating >= 4 || track.PlayCountPercentile >= 0.65
				return favorite && track.HasLastPlayed && track.DaysSinceLastPlayed >= 180
			},
			score: func(track features.TrackFeatures) float64 {
				return 1.5*preferenceScore(track) +
					1.4*longTailScore(track.DaysSinceLastPlayed, 180, 75) +
					0.6*track.StabilityScore
			},
			carryover:  -0.5,
			weekJitter: 0.25,
		},
		{
			name: "Rising This Week",
			eligible: func(track features.TrackFeatures) bool {
				return track.HasLastPlayed && track.DaysSinceLastPlayed <= 14 && track.PlayCountDelta > 0
			},
			score: func(track features.TrackFeatures) float64 {
				return 1.8*deltaScore(track.PlayCountDelta) +
					1.3*track.RecencyTrendScore +
					0.6*track.PlayCountPercentile +
					0.3*preferenceScore(track)
			},
			carryover:  0.1,
			weekJitter: 0.05,
		},
		{
			name: "Deep Cuts",
			eligible: func(track features.TrackFeatures) bool {
				return track.ArtistAffinity >= 0.35 && track.PlayCountPercentile <= 0.55
			},
			score: func(track features.TrackFeatures) float64 {
				return 1.7*track.ArtistAffinity +
					1.4*track.NoveltyScore +
					0.5*preferenceScore(track) +
					0.4*freshnessCurve(track.DaysSinceAdded, 0, 365)
			},
			carryover:  -0.35,
			weekJitter: 0.35,
		},
		{
			name: "Quick Mix",
			eligible: func(track features.TrackFeatures) bool {
				return track.Track.Duration > 0 && track.Track.Duration <= quickTrackSeconds
			},
			score: func(track features.TrackFeatures) float64 {
				return g.engine.BaseScore(track) + 0.5*preferenceScore(track) - 0.4*track.RepeatFatigueScore
			},
			carryover:  -0.15,
			weekJitter: 0.65,
		},
		{
			name: "Longform",
			eligible: func(track features.TrackFeatures) bool {
				return track.Track.Duration >= longformSeconds
			},
			score: func(track features.TrackFeatures) float64 {
				return g.engine.BaseScore(track) + 0.6*preferenceScore(track) - 0.3*track.RepeatFatigueScore
			},
			carryover:  -0.1,
			weekJitter: 0.5,
		},
	}
}

func (g *Generator) similarPlaylist(
	name string,
	dataset features.Dataset,
	previous *state.HistoryState,
	now time.Time,
	seeds []features.TrackFeatures,
	excludeSeedArtists bool,
	carryover float64,
	weekJitter float64,
	scorer func(track features.TrackFeatures, similarityScore, genreMatch float64) float64,
) []features.TrackFeatures {
	if len(seeds) == 0 {
		return nil
	}

	seedVectors := make([][]float64, 0, min(len(seeds), 10))
	seedIDs := make(map[string]struct{}, len(seeds))
	seedArtists := make(map[string]struct{}, len(seeds))
	seedGenreCounts := make(map[string]int, len(seeds))
	for index, seed := range seeds {
		seedIDs[seed.Track.ID] = struct{}{}
		seedArtists[canonicalKey(seed.Track.Artist)] = struct{}{}
		if genre := canonicalKey(seed.Track.Genre); genre != "__unknown__" {
			seedGenreCounts[genre]++
		}
		if index < 10 {
			seedVectors = append(seedVectors, seed.SimilarityVector)
		}
	}

	centroid := similarity.Centroid(seedVectors)
	if len(centroid) == 0 {
		return nil
	}

	scored := make([]scoredTrack, 0, len(dataset.Items))
	for _, track := range dataset.Items {
		if _, isSeed := seedIDs[track.Track.ID]; isSeed {
			continue
		}
		_, duplicateArtist := seedArtists[canonicalKey(track.Track.Artist)]
		if excludeSeedArtists && duplicateArtist {
			continue
		}

		genreMatch := 0.0
		if matches := seedGenreCounts[canonicalKey(track.Track.Genre)]; matches > 0 {
			genreMatch = float64(matches) / float64(len(seeds))
		}
		score := scorer(track, similarity.CosineSimilarity(track.SimilarityVector, centroid), genreMatch)
		if duplicateArtist {
			score -= 0.25
		}
		score += g.playlistCarryover(previous, name, track.Track.ID, carryover)
		score += centeredWeeklyJitter(name, track.Track.ID, now) * weekJitter
		scored = append(scored, scoredTrack{track: track, score: score})
	}

	sortScored(scored)
	return g.finalize(name, scored)
}

func (g *Generator) rankRecipe(currentRecipe recipe, items []features.TrackFeatures, previous *state.HistoryState, now time.Time) []features.TrackFeatures {
	scored := make([]scoredTrack, 0, len(items))
	for _, track := range items {
		if currentRecipe.eligible != nil && !currentRecipe.eligible(track) {
			continue
		}
		score := currentRecipe.score(track)
		score += g.playlistCarryover(previous, currentRecipe.name, track.Track.ID, currentRecipe.carryover)
		score += centeredWeeklyJitter(currentRecipe.name, track.Track.ID, now) * currentRecipe.weekJitter
		scored = append(scored, scoredTrack{track: track, score: score})
	}

	sortScored(scored)
	return g.finalize(currentRecipe.name, scored)
}

func sortScored(scored []scoredTrack) {
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score == scored[j].score {
			return scored[i].track.Track.ID < scored[j].track.Track.ID
		}
		return scored[i].score > scored[j].score
	})
}

func (g *Generator) finalize(name string, scored []scoredTrack) []features.TrackFeatures {
	result := limitDiversity(scored, g.size, defaultArtistLimit, defaultAlbumLimit)
	if len(result) < min(g.size, g.minBackfill) {
		result = limitDiversity(scored, g.size, maxBackfillLimit, maxBackfillLimit)
	}

	g.logger.Printf("playlist %q candidates=%d selected=%d", name, len(scored), len(result))
	return result
}

func (g *Generator) playlistCarryover(previous *state.HistoryState, playlistName, trackID string, adjustment float64) float64 {
	if previous == nil || !previous.PlaylistContains(playlistName, trackID) {
		return 0
	}

	return adjustment
}

func centeredWeeklyJitter(playlistName, trackID string, now time.Time) float64 {
	year, week := now.ISOWeek()
	hasher := fnv.New64a()
	_, _ = hasher.Write([]byte(fmt.Sprintf("%s\x00%s\x00%d\x00%d", playlistName, trackID, year, week)))
	value := float64(hasher.Sum64()%1_000_000) / 1_000_000
	return value - 0.5
}

func toTracks(items []features.TrackFeatures) []model.Track {
	tracks := make([]model.Track, 0, len(items))
	for _, item := range items {
		tracks = append(tracks, item.Track)
	}

	return tracks
}

func limitDiversity(items []scoredTrack, size, maxPerArtist, maxPerAlbum int) []features.TrackFeatures {
	result := make([]features.TrackFeatures, 0, min(len(items), size))
	artistCounts := make(map[string]int)
	albumCounts := make(map[string]int)

	for _, item := range items {
		if len(result) >= size {
			break
		}

		artistKey := diversityKey(item.track.Track.Artist, item.track.Track.ID)
		albumKey := diversityKey(item.track.Track.Album, item.track.Track.ID)
		if artistCounts[artistKey] >= maxPerArtist || albumCounts[albumKey] >= maxPerAlbum {
			continue
		}

		artistCounts[artistKey]++
		albumCounts[albumKey]++
		result = append(result, item.track)
	}

	return result
}

func canonicalKey(value string) string {
	trimmed := strings.TrimSpace(strings.ToLower(value))
	if trimmed == "" {
		return "__unknown__"
	}
	return trimmed
}

func diversityKey(value, trackID string) string {
	key := canonicalKey(value)
	if key == "__unknown__" || key == "unknown artist" || key == "unknown album" {
		return key + ":" + trackID
	}
	return key
}

func windowScore(value, start, end, softness float64) float64 {
	if value >= start && value <= end {
		return 1
	}
	if value < start {
		return math.Exp(-(start - value) / softness)
	}
	return math.Exp(-(value - end) / softness)
}

func longTailScore(value, start, softness float64) float64 {
	if value <= start {
		return 0
	}
	return 1 - math.Exp(-(value-start)/softness)
}

func freshnessCurve(value, start, end float64) float64 {
	if value <= start {
		return 1
	}
	if value >= end || end <= start {
		return 0
	}
	return 1 - (value-start)/(end-start)
}

func preferenceScore(track features.TrackFeatures) float64 {
	rating := clamp01(float64(track.Track.Rating) / 5.0)
	starred := boolScore(track.Track.Starred)
	return clamp01(0.5*track.PlayCountPercentile + 0.3*rating + 0.2*starred)
}

func deltaScore(value float64) float64 {
	return clamp01(value / 5.0)
}

func boolScore(value bool) float64 {
	if value {
		return 1
	}
	return 0
}

func clamp01(value float64) float64 {
	switch {
	case value < 0:
		return 0
	case value > 1:
		return 1
	default:
		return value
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
