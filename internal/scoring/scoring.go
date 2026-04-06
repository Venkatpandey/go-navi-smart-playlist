package scoring

import (
	"math"

	"go-navi-smart-playlist/internal/config"
	"go-navi-smart-playlist/internal/features"
)

type Engine struct {
	weights config.Weights
}

func New(weights config.Weights) *Engine {
	return &Engine{weights: weights}
}

func (e *Engine) BaseScore(track features.TrackFeatures) float64 {
	playCountTerm := math.Log(float64(track.Track.PlayCount) + 1)
	if track.Track.PlayCount == 0 {
		playCountTerm += 0.35
	}

	recencyScore := math.Exp(-track.DaysSinceLastPlayed / e.weights.DecayDays)
	freshnessScore := math.Exp(-track.DaysSinceAdded / e.weights.DecayDays)
	ratingScore := clamp01(float64(track.Track.Rating) / 5.0)

	return e.weights.PlayCount*playCountTerm +
		e.weights.Recency*recencyScore +
		e.weights.Freshness*freshnessScore +
		0.7*track.NoveltyScore +
		0.6*track.RecencyTrendScore +
		0.3*track.StabilityScore +
		0.25*ratingScore +
		0.2*boolFloat(track.Track.Starred) -
		0.75*track.RepeatFatigueScore -
		0.2*track.ArtistSaturation -
		0.15*track.AlbumSaturation
}

func boolFloat(value bool) float64 {
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
