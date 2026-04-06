package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"
)

type HistoryState struct {
	Version   int                         `json:"version"`
	UpdatedAt time.Time                   `json:"updatedAt"`
	Tracks    map[string]TrackSnapshot    `json:"tracks"`
	Playlists map[string]PlaylistSnapshot `json:"playlists"`
}

type TrackSnapshot struct {
	ID         string                 `json:"id"`
	PlayCount  int                    `json:"playCount"`
	LastPlayed time.Time              `json:"lastPlayed"`
	Created    time.Time              `json:"created"`
	Artist     string                 `json:"artist"`
	Album      string                 `json:"album"`
	SeenCount  int                    `json:"seenCount"`
	LastSeenAt time.Time              `json:"lastSeenAt"`
	Derived    DerivedFeatureSnapshot `json:"derived"`
}

type DerivedFeatureSnapshot struct {
	PlayCountPercentile float64   `json:"playCountPercentile"`
	DaysSinceLastPlayed float64   `json:"daysSinceLastPlayed"`
	DaysSinceAdded      float64   `json:"daysSinceAdded"`
	PlayCountDelta      float64   `json:"playCountDelta"`
	RecencyTrend        float64   `json:"recencyTrend"`
	RepeatFatigue       float64   `json:"repeatFatigue"`
	ArtistSaturation    float64   `json:"artistSaturation"`
	AlbumSaturation     float64   `json:"albumSaturation"`
	NoveltyScore        float64   `json:"noveltyScore"`
	StabilityScore      float64   `json:"stabilityScore"`
	Vector              []float64 `json:"vector,omitempty"`
}

type PlaylistSnapshot struct {
	TrackIDs []string `json:"trackIds"`
}

type Store struct {
	path    string
	enabled bool
	logger  *log.Logger
}

func NewStore(path string, enabled bool, logger *log.Logger) *Store {
	return &Store{
		path:    path,
		enabled: enabled,
		logger:  logger,
	}
}

func NewHistoryState() *HistoryState {
	return &HistoryState{
		Version:   1,
		Tracks:    map[string]TrackSnapshot{},
		Playlists: map[string]PlaylistSnapshot{},
	}
}

func (s *Store) Load() (*HistoryState, error) {
	if !s.enabled {
		return NewHistoryState(), nil
	}

	file, err := os.Open(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return NewHistoryState(), nil
		}

		return NewHistoryState(), fmt.Errorf("open state file: %w", err)
	}
	defer file.Close()

	payload := NewHistoryState()
	if err := json.NewDecoder(file).Decode(payload); err != nil {
		if errors.Is(err, io.EOF) {
			return payload, nil
		}

		return NewHistoryState(), fmt.Errorf("decode state file: %w", err)
	}

	if payload.Tracks == nil {
		payload.Tracks = map[string]TrackSnapshot{}
	}
	if payload.Playlists == nil {
		payload.Playlists = map[string]PlaylistSnapshot{}
	}

	return payload, nil
}

func (s *Store) Save(payload *HistoryState) error {
	if !s.enabled || payload == nil {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}

	file, err := os.Create(s.path)
	if err != nil {
		return fmt.Errorf("create state file: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(payload); err != nil {
		return fmt.Errorf("encode state file: %w", err)
	}

	return nil
}

func (h *HistoryState) PlaylistContains(name, trackID string) bool {
	if h == nil {
		return false
	}

	playlist, ok := h.Playlists[name]
	if !ok {
		return false
	}

	for _, id := range playlist.TrackIDs {
		if id == trackID {
			return true
		}
	}

	return false
}
