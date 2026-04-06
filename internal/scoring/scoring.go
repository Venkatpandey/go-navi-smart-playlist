package scoring

import (
	"math"
	"time"

	"go-navi-smart-playlist/internal/config"
	"go-navi-smart-playlist/internal/model"
)

type Engine struct {
	weights config.Weights
}

func New(weights config.Weights) *Engine {
	return &Engine{weights: weights}
}

func (e *Engine) Score(track model.Track, now time.Time) float64 {
	recencyDays := daysSince(track.LastPlayed, now, 365*10)
	freshnessDays := daysSince(track.Created, now, 365*10)

	playCountTerm := math.Log(float64(track.PlayCount) + 1)
	if track.PlayCount == 0 {
		playCountTerm += 0.35
	}

	recencyScore := math.Exp(-recencyDays / e.weights.DecayDays)
	freshnessScore := math.Exp(-freshnessDays / e.weights.DecayDays)

	return e.weights.PlayCount*playCountTerm +
		e.weights.Recency*recencyScore +
		e.weights.Freshness*freshnessScore
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
