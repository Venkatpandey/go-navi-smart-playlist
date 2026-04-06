package features

import (
	"log"
	"math"
	"slices"
	"strings"
	"time"

	"go-navi-smart-playlist/internal/model"
	"go-navi-smart-playlist/internal/state"
)

type TrackFeatures struct {
	Track               model.Track
	PlayCountPercentile float64
	DaysSinceLastPlayed float64
	DaysSinceAdded      float64
	PlayCountDelta      float64
	RecencyTrendScore   float64
	RepeatFatigueScore  float64
	ArtistSaturation    float64
	AlbumSaturation     float64
	NoveltyScore        float64
	StabilityScore      float64
	HasLastPlayed       bool
	SimilarityVector    []float64
}

type Dataset struct {
	Items []TrackFeatures
	Stats Stats
	byID  map[string]TrackFeatures
}

type Stats struct {
	TotalTracks          int
	TracksWithLastPlayed int
	TracksWithPlayCount  int
	MaxArtistCount       int
	MaxAlbumCount        int
}

type Builder struct {
	logger *log.Logger
}

func NewBuilder(logger *log.Logger) *Builder {
	return &Builder{logger: logger}
}

func (b *Builder) Build(tracks []model.Track, previous *state.HistoryState, now time.Time) Dataset {
	playCounts := make([]int, 0, len(tracks))
	artistCounts := make(map[string]int)
	albumCounts := make(map[string]int)
	stats := Stats{TotalTracks: len(tracks)}

	for _, track := range tracks {
		playCounts = append(playCounts, track.PlayCount)
		artistCounts[groupKey(track.Artist)]++
		albumCounts[groupKey(track.Album)]++
		if !track.LastPlayed.IsZero() {
			stats.TracksWithLastPlayed++
		}
		if track.PlayCount > 0 {
			stats.TracksWithPlayCount++
		}
	}

	slices.Sort(playCounts)
	stats.MaxArtistCount = maxMapValue(artistCounts)
	stats.MaxAlbumCount = maxMapValue(albumCounts)

	items := make([]TrackFeatures, 0, len(tracks))
	byID := make(map[string]TrackFeatures, len(tracks))
	for _, track := range tracks {
		var previousSnapshot state.TrackSnapshot
		var seenBefore bool
		if previous != nil {
			previousSnapshot, seenBefore = previous.Tracks[track.ID]
		}

		playPercentile := percentile(track.PlayCount, playCounts)
		daysSinceLastPlayed := daysSince(track.LastPlayed, now, 3650)
		daysSinceAdded := daysSince(track.Created, now, 3650)
		playDelta := math.Max(0, float64(track.PlayCount-previousSnapshot.PlayCount))
		recencyFactor := math.Exp(-daysSinceLastPlayed / 21)
		freshnessFactor := math.Exp(-daysSinceAdded / 30)
		recencyTrend := clamp01(playDelta/5.0)*recencyFactor + clamp01(recencyFactor*0.25)
		repeatFatigue := clamp01(recencyFactor*0.8 + clamp01(playDelta/6.0)*0.6)
		artistSaturation := normalizeCount(artistCounts[groupKey(track.Artist)], stats.MaxArtistCount)
		albumSaturation := normalizeCount(albumCounts[groupKey(track.Album)], stats.MaxAlbumCount)
		noveltyScore := clamp01((1-playPercentile)*0.7 + freshnessFactor*0.3)
		stabilityScore := clamp01(float64(previousSnapshot.SeenCount) / 5.0)
		if !seenBefore {
			stabilityScore *= 0.25
		}

		vector := []float64{
			playPercentile,
			clamp01(1 - daysSinceLastPlayed/365.0),
			clamp01(1 - daysSinceAdded/365.0),
			clamp01(playDelta / 5.0),
			recencyTrend,
			1 - repeatFatigue,
			1 - artistSaturation,
			1 - albumSaturation,
			noveltyScore,
			stabilityScore,
			clamp01(float64(track.Rating) / 5.0),
			boolFloat(track.Starred),
		}

		item := TrackFeatures{
			Track:               track,
			PlayCountPercentile: playPercentile,
			DaysSinceLastPlayed: daysSinceLastPlayed,
			DaysSinceAdded:      daysSinceAdded,
			PlayCountDelta:      playDelta,
			RecencyTrendScore:   clamp01(recencyTrend),
			RepeatFatigueScore:  repeatFatigue,
			ArtistSaturation:    artistSaturation,
			AlbumSaturation:     albumSaturation,
			NoveltyScore:        noveltyScore,
			StabilityScore:      stabilityScore,
			HasLastPlayed:       !track.LastPlayed.IsZero(),
			SimilarityVector:    vector,
		}

		items = append(items, item)
		byID[item.Track.ID] = item
	}

	if b.logger != nil {
		b.logger.Printf(
			"feature dataset: total=%d with_last_played=%d with_play_count=%d max_artist=%d max_album=%d",
			stats.TotalTracks,
			stats.TracksWithLastPlayed,
			stats.TracksWithPlayCount,
			stats.MaxArtistCount,
			stats.MaxAlbumCount,
		)
	}

	return Dataset{
		Items: items,
		Stats: stats,
		byID:  byID,
	}
}

func (d Dataset) Get(id string) (TrackFeatures, bool) {
	item, ok := d.byID[id]
	return item, ok
}

func boolFloat(value bool) float64 {
	if value {
		return 1
	}
	return 0
}

func percentile(value int, sorted []int) float64 {
	if len(sorted) == 0 {
		return 0
	}
	index := slices.Index(sorted, value)
	if index < 0 {
		return 0
	}
	if len(sorted) == 1 {
		return 1
	}
	return float64(index) / float64(len(sorted)-1)
}

func daysSince(when, now time.Time, fallback float64) float64 {
	if when.IsZero() {
		return fallback
	}

	days := now.Sub(when).Hours() / 24
	if days < 0 {
		return 0
	}

	return days
}

func normalizeCount(value, maxValue int) float64 {
	if maxValue <= 1 {
		return 0
	}
	return clamp01(float64(value-1) / float64(maxValue-1))
}

func maxMapValue(items map[string]int) int {
	maxValue := 0
	for _, value := range items {
		if value > maxValue {
			maxValue = value
		}
	}
	return maxValue
}

func groupKey(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "__unknown__"
	}
	return strings.ToLower(trimmed)
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
