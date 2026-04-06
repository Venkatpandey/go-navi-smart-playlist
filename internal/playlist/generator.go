package playlist

import (
	"log"
	"sort"
	"time"

	"go-navi-smart-playlist/internal/config"
	"go-navi-smart-playlist/internal/model"
	"go-navi-smart-playlist/internal/scoring"
)

type Definition struct {
	Name   string
	Tracks []model.Track
}

type Generator struct {
	size   int
	engine *scoring.Engine
	logger *log.Logger
}

type scoredTrack struct {
	track model.Track
	score float64
}

func NewGenerator(cfg config.Config, logger *log.Logger) *Generator {
	return &Generator{
		size:   cfg.PlaylistSize,
		engine: scoring.New(cfg.Weights),
		logger: logger,
	}
}

func (g *Generator) Generate(tracks []model.Track, now time.Time) []Definition {
	definitions := []Definition{
		{Name: "Discover Weekly", Tracks: g.discoverWeekly(tracks, now)},
		{Name: "Rediscover", Tracks: g.rediscover(tracks, now)},
		{Name: "Top This Month", Tracks: g.topThisMonth(tracks, now)},
	}

	for _, definition := range definitions {
		g.logger.Printf("generated playlist %q with %d tracks", definition.Name, len(definition.Tracks))
	}

	return definitions
}

func (g *Generator) discoverWeekly(tracks []model.Track, now time.Time) []model.Track {
	var candidates []scoredTrack
	for _, track := range tracks {
		daysSincePlayed := daysSince(track.LastPlayed, now)
		if track.PlayCount > 8 {
			continue
		}
		if !track.LastPlayed.IsZero() && daysSincePlayed < 21 {
			continue
		}

		score := g.engine.Score(track, now)
		if track.PlayCount == 0 {
			score += 0.5
		}
		if track.Starred {
			score += 0.25
		}

		candidates = append(candidates, scoredTrack{track: track, score: score})
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})

	return limitPerArtist(toTracks(candidates), g.size, 2)
}

func (g *Generator) rediscover(tracks []model.Track, now time.Time) []model.Track {
	var candidates []scoredTrack
	for _, track := range tracks {
		daysSincePlayed := daysSince(track.LastPlayed, now)
		if track.PlayCount < 5 {
			continue
		}
		if track.LastPlayed.IsZero() || daysSincePlayed < 60 || daysSincePlayed > 90 {
			continue
		}

		score := g.engine.Score(track, now) + float64(track.PlayCount)*0.1
		candidates = append(candidates, scoredTrack{track: track, score: score})
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})

	return limitPerArtist(toTracks(candidates), g.size, 2)
}

func (g *Generator) topThisMonth(tracks []model.Track, now time.Time) []model.Track {
	var candidates []model.Track
	for _, track := range tracks {
		if track.LastPlayed.IsZero() {
			continue
		}
		if daysSince(track.LastPlayed, now) > 30 {
			continue
		}

		candidates = append(candidates, track)
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].PlayCount == candidates[j].PlayCount {
			return candidates[i].LastPlayed.After(candidates[j].LastPlayed)
		}
		return candidates[i].PlayCount > candidates[j].PlayCount
	})

	return limitPerArtist(candidates, g.size, 2)
}

func toTracks(items []scoredTrack) []model.Track {
	tracks := make([]model.Track, 0, len(items))
	for _, item := range items {
		tracks = append(tracks, item.track)
	}

	return tracks
}

func limitPerArtist(tracks []model.Track, size, maxPerArtist int) []model.Track {
	result := make([]model.Track, 0, min(len(tracks), size))
	artistCounts := make(map[string]int)

	for _, track := range tracks {
		if len(result) >= size {
			break
		}

		artistKey := track.Artist
		if artistKey == "" {
			artistKey = "__unknown__"
		}
		if artistCounts[artistKey] >= maxPerArtist {
			continue
		}

		artistCounts[artistKey]++
		result = append(result, track)
	}

	return result
}

func min(a, b int) int {
	if a < b {
		return a
	}

	return b
}

func daysSince(when, now time.Time) float64 {
	if when.IsZero() {
		return 3650
	}

	days := now.Sub(when).Hours() / 24
	if days < 0 {
		return 0
	}

	return days
}
