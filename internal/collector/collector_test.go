package collector

import (
	"testing"

	"go-navi-smart-playlist/internal/navidrome"
)

func TestNormalizeTrackIncludesPlaylistMetadata(t *testing.T) {
	track := normalizeTrack(navidrome.TrackPayload{
		ID:       "track-1",
		Title:    " Title ",
		Artist:   " Artist ",
		Album:    " Album ",
		Genre:    " Jazz ",
		Duration: 612,
	})

	if track.Genre != "Jazz" || track.Duration != 612 {
		t.Fatalf("metadata not normalized: %+v", track)
	}
}

func TestNormalizeTrackClampsInvalidNumericMetadata(t *testing.T) {
	track := normalizeTrack(navidrome.TrackPayload{ID: "track-1", Duration: -20, PlayCount: -3})
	if track.Duration != 0 || track.PlayCount != 0 {
		t.Fatalf("numeric metadata not clamped: %+v", track)
	}
}
