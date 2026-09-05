package scoring

import (
	"testing"

	"go-navi-smart-playlist/internal/config"
	"go-navi-smart-playlist/internal/features"
	"go-navi-smart-playlist/internal/model"
)

func TestBaseScoreUsesNormalizedPlayCount(t *testing.T) {
	engine := New(config.Weights{PlayCount: 1, Recency: 1, Freshness: 1, DecayDays: 45})
	base := features.TrackFeatures{PlayCountPercentile: 0.8, DaysSinceLastPlayed: 30, DaysSinceAdded: 300}

	low := base
	low.Track = model.Track{PlayCount: 10}
	high := base
	high.Track = model.Track{PlayCount: 1_000_000}

	if engine.BaseScore(low) != engine.BaseScore(high) {
		t.Fatalf("raw play count magnitude must not dominate normalized score")
	}
}
