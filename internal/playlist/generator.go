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
		{Name: "Hidden Gems", Tracks: g.hiddenGems(tracks, now)},
		{Name: "Long Time No See", Tracks: g.longTimeNoSee(tracks, now)},
		{Name: "Comfort Shuffle", Tracks: g.comfortShuffle(tracks, now)},
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

	return limitDiversity(toTracks(candidates), g.size, 3, 3)
}

func (g *Generator) rediscover(tracks []model.Track, now time.Time) []model.Track {
	var candidates []scoredTrack
	for _, track := range tracks {
		daysSincePlayed := daysSince(track.LastPlayed, now)
		if track.PlayCount < 2 {
			continue
		}
		if track.LastPlayed.IsZero() || daysSincePlayed < 45 || daysSincePlayed > 180 {
			continue
		}

		score := g.engine.Score(track, now) + float64(track.PlayCount)*0.1
		candidates = append(candidates, scoredTrack{track: track, score: score})
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})

	return limitDiversity(toTracks(candidates), g.size, 3, 3)
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

	return limitDiversity(candidates, g.size, 3, 3)
}

func (g *Generator) hiddenGems(tracks []model.Track, now time.Time) []model.Track {
	var candidates []scoredTrack
	for _, track := range tracks {
		if track.PlayCount > 6 {
			continue
		}
		if !track.LastPlayed.IsZero() && daysSince(track.LastPlayed, now) < 30 {
			continue
		}

		score := g.engine.Score(track, now)
		if track.Rating >= 4 {
			score += 1.0
		}
		if track.Starred {
			score += 0.75
		}
		if track.PlayCount <= 2 {
			score += 0.35
		}

		candidates = append(candidates, scoredTrack{track: track, score: score})
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})

	return limitDiversity(toTracks(candidates), g.size, 3, 3)
}

func (g *Generator) longTimeNoSee(tracks []model.Track, now time.Time) []model.Track {
	var candidates []scoredTrack
	for _, track := range tracks {
		if track.PlayCount < 1 || track.LastPlayed.IsZero() {
			continue
		}

		days := daysSince(track.LastPlayed, now)
		if days < 120 {
			continue
		}

		score := g.engine.Score(track, now) + float64(track.PlayCount)*0.15
		if track.Rating >= 4 {
			score += 0.5
		}
		if track.Starred {
			score += 0.5
		}

		candidates = append(candidates, scoredTrack{track: track, score: score})
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})

	return limitDiversity(toTracks(candidates), g.size, 3, 3)
}

func (g *Generator) comfortShuffle(tracks []model.Track, now time.Time) []model.Track {
	var candidates []scoredTrack
	for _, track := range tracks {
		if track.PlayCount < 3 {
			continue
		}

		days := daysSince(track.LastPlayed, now)
		if !track.LastPlayed.IsZero() && days < 7 {
			continue
		}
		if !track.LastPlayed.IsZero() && days > 180 {
			continue
		}

		score := g.engine.Score(track, now) + float64(track.PlayCount)*0.2
		if track.Rating >= 4 {
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

	return limitDiversity(toTracks(candidates), g.size, 3, 3)
}

func toTracks(items []scoredTrack) []model.Track {
	tracks := make([]model.Track, 0, len(items))
	for _, item := range items {
		tracks = append(tracks, item.track)
	}

	return tracks
}

func limitDiversity(tracks []model.Track, size, maxPerArtist, maxPerAlbum int) []model.Track {
	result := make([]model.Track, 0, min(len(tracks), size))
	artistCounts := make(map[string]int)
	albumCounts := make(map[string]int)

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

		albumKey := track.Album
		if albumKey == "" {
			albumKey = "__unknown__"
		}
		if albumCounts[albumKey] >= maxPerAlbum {
			continue
		}

		artistCounts[artistKey]++
		albumCounts[albumKey]++
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
