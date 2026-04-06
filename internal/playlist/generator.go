package playlist

import (
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

func NewGenerator(cfg config.Config, logger *log.Logger) *Generator {
	return &Generator{
		size:        cfg.PlaylistSize,
		minBackfill: cfg.MinBackfill,
		engine:      scoring.New(cfg.Weights),
		logger:      logger,
	}
}

func (g *Generator) Generate(dataset features.Dataset, previous *state.HistoryState, now time.Time) []Definition {
	discoverWeekly := g.rankPlaylist("Discover Weekly", dataset.Items, previous, func(track features.TrackFeatures) float64 {
		return g.engine.BaseScore(track) +
			1.6*track.NoveltyScore +
			0.6*freshnessCurve(track.DaysSinceAdded, 0, 120) +
			0.4*windowScore(track.DaysSinceLastPlayed, 14, 120, 35) -
			0.7*track.PlayCountPercentile
	})

	rediscover := g.rankPlaylist("Rediscover", dataset.Items, previous, func(track features.TrackFeatures) float64 {
		return g.engine.BaseScore(track) +
			1.4*windowScore(track.DaysSinceLastPlayed, 45, 180, 40) +
			0.9*track.PlayCountPercentile +
			0.4*track.StabilityScore -
			0.3*track.NoveltyScore
	})

	topThisMonth := g.rankPlaylist("Top This Month", dataset.Items, previous, func(track features.TrackFeatures) float64 {
		return g.engine.BaseScore(track) +
			1.8*windowScore(track.DaysSinceLastPlayed, 0, 30, 12) +
			0.9*track.RecencyTrendScore +
			0.4*track.PlayCountDelta +
			0.5*track.PlayCountPercentile
	})

	hiddenGems := g.rankPlaylist("Hidden Gems", dataset.Items, previous, func(track features.TrackFeatures) float64 {
		return g.engine.BaseScore(track) +
			1.5*track.NoveltyScore +
			0.8*(1-track.PlayCountPercentile) +
			0.35*boolScore(track.Track.Starred) +
			0.35*clamp01(float64(track.Track.Rating)/5.0) +
			0.3*windowScore(track.DaysSinceLastPlayed, 21, 240, 60)
	})

	longTimeNoSee := g.rankPlaylist("Long Time No See", dataset.Items, previous, func(track features.TrackFeatures) float64 {
		return g.engine.BaseScore(track) +
			1.6*longTailScore(track.DaysSinceLastPlayed, 120, 45) +
			0.7*track.PlayCountPercentile +
			0.4*track.StabilityScore -
			0.2*track.NoveltyScore
	})

	comfortShuffle := g.rankPlaylist("Comfort Shuffle", dataset.Items, previous, func(track features.TrackFeatures) float64 {
		return g.engine.BaseScore(track) +
			1.5*windowScore(track.DaysSinceLastPlayed, 7, 180, 45) +
			1.0*track.PlayCountPercentile +
			0.5*track.StabilityScore -
			0.4*track.RepeatFatigueScore
	})

	moreLikeHiddenGems := g.similarPlaylist(
		"More Like Hidden Gems",
		dataset,
		previous,
		hiddenGems,
		func(track features.TrackFeatures, similarityScore float64) float64 {
			return 1.5*similarityScore + g.engine.BaseScore(track) + 0.8*track.NoveltyScore
		},
	)

	artistAdjacentComfort := g.similarPlaylist(
		"Artist Adjacent Comfort",
		dataset,
		previous,
		comfortShuffle,
		func(track features.TrackFeatures, similarityScore float64) float64 {
			return 1.5*similarityScore + g.engine.BaseScore(track) + 0.5*track.StabilityScore + 0.4*track.PlayCountPercentile
		},
	)

	definitions := []Definition{
		{Name: "Discover Weekly", Tracks: toTracks(discoverWeekly)},
		{Name: "Rediscover", Tracks: toTracks(rediscover)},
		{Name: "Top This Month", Tracks: toTracks(topThisMonth)},
		{Name: "Hidden Gems", Tracks: toTracks(hiddenGems)},
		{Name: "Long Time No See", Tracks: toTracks(longTimeNoSee)},
		{Name: "Comfort Shuffle", Tracks: toTracks(comfortShuffle)},
		{Name: "More Like Hidden Gems", Tracks: toTracks(moreLikeHiddenGems)},
		{Name: "Artist Adjacent Comfort", Tracks: toTracks(artistAdjacentComfort)},
	}

	for _, definition := range definitions {
		g.logger.Printf("generated playlist %q with %d tracks", definition.Name, len(definition.Tracks))
	}

	return definitions
}

func (g *Generator) similarPlaylist(
	name string,
	dataset features.Dataset,
	previous *state.HistoryState,
	seeds []features.TrackFeatures,
	scorer func(track features.TrackFeatures, similarityScore float64) float64,
) []features.TrackFeatures {
	if len(seeds) == 0 {
		return nil
	}

	seedVectors := make([][]float64, 0, min(len(seeds), 10))
	seedIDs := make(map[string]struct{}, len(seeds))
	seedArtists := make(map[string]struct{}, len(seeds))
	for index, seed := range seeds {
		if index >= 10 {
			break
		}
		seedVectors = append(seedVectors, seed.SimilarityVector)
		seedIDs[seed.Track.ID] = struct{}{}
		seedArtists[canonicalKey(seed.Track.Artist)] = struct{}{}
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

		score := scorer(track, similarity.CosineSimilarity(track.SimilarityVector, centroid))
		if _, duplicateArtist := seedArtists[canonicalKey(track.Track.Artist)]; duplicateArtist {
			score -= 0.25
		}
		score += g.previousPlaylistBoost(previous, name, track.Track.ID)

		scored = append(scored, scoredTrack{track: track, score: score})
	}

	sort.SliceStable(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	return g.finalize(name, scored)
}

func (g *Generator) rankPlaylist(
	name string,
	items []features.TrackFeatures,
	previous *state.HistoryState,
	scorer func(track features.TrackFeatures) float64,
) []features.TrackFeatures {
	scored := make([]scoredTrack, 0, len(items))
	for _, track := range items {
		score := scorer(track) + g.previousPlaylistBoost(previous, name, track.Track.ID)
		scored = append(scored, scoredTrack{track: track, score: score})
	}

	sort.SliceStable(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	return g.finalize(name, scored)
}

func (g *Generator) finalize(name string, scored []scoredTrack) []features.TrackFeatures {
	result := limitDiversity(scored, g.size, defaultArtistLimit, defaultAlbumLimit)
	if len(result) < min(g.size, g.minBackfill) {
		result = limitDiversity(scored, g.size, maxBackfillLimit, maxBackfillLimit)
	}

	g.logger.Printf("playlist %q candidates=%d selected=%d", name, len(scored), len(result))
	return result
}

func (g *Generator) previousPlaylistBoost(previous *state.HistoryState, playlistName, trackID string) float64 {
	if previous == nil || !previous.PlaylistContains(playlistName, trackID) {
		return 0
	}

	return 0.18
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

		artistKey := canonicalKey(item.track.Track.Artist)
		albumKey := canonicalKey(item.track.Track.Album)
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
	if value >= start {
		return 1 - math.Exp(-(value-start)/softness)
	}
	return math.Exp(-(start - value) / softness)
}

func freshnessCurve(value, start, end float64) float64 {
	return windowScore(value, start, end, 45)
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
