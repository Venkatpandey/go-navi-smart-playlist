package config

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	defaultPlaylistSize  = 50
	defaultAlbumPageSize = 200
	defaultDecayDays     = 45
	defaultRunTimeout    = 15 * time.Minute
	defaultClientName    = "go-smart-playlist"
	defaultAPIVersion    = "1.16.1"
	defaultStateDir      = "/tmp/go-smart-playlist"
	defaultStateFileName = "state.json"
	defaultBackfillSize  = 20
)

type Config struct {
	BaseURL       string
	Username      string
	Password      string
	PlaylistSize  int
	AlbumPageSize int
	Weights       Weights
	RunTimeout    time.Duration
	ClientName    string
	APIVersion    string
	StateFile     string
	EnableState   bool
	MinBackfill   int
}

type Weights struct {
	PlayCount float64
	Recency   float64
	Freshness float64
	DecayDays float64
}

func Load() (Config, error) {
	playlistSize, err := getInt("PLAYLIST_SIZE", defaultPlaylistSize)
	if err != nil {
		return Config{}, err
	}
	albumPageSize, err := getInt("ALBUM_PAGE_SIZE", defaultAlbumPageSize)
	if err != nil {
		return Config{}, err
	}
	runTimeout, err := getDuration("RUN_TIMEOUT", defaultRunTimeout)
	if err != nil {
		return Config{}, err
	}
	enableState, err := getBool("ENABLE_STATE_CACHE", true)
	if err != nil {
		return Config{}, err
	}
	minBackfill, err := getInt("MIN_CANDIDATE_BACKFILL", defaultBackfillSize)
	if err != nil {
		return Config{}, err
	}
	playCountWeight, err := getFloat("SCORE_WEIGHT_PLAYCOUNT", 1.0)
	if err != nil {
		return Config{}, err
	}
	recencyWeight, err := getFloat("SCORE_WEIGHT_RECENCY", 2.0)
	if err != nil {
		return Config{}, err
	}
	freshnessWeight, err := getFloat("SCORE_WEIGHT_FRESHNESS", 1.5)
	if err != nil {
		return Config{}, err
	}
	decayDays, err := getFloat("SCORE_DECAY_DAYS", defaultDecayDays)
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		BaseURL:       strings.TrimRight(strings.TrimSpace(os.Getenv("NAVIDROME_URL")), "/"),
		Username:      strings.TrimSpace(os.Getenv("NAVIDROME_USER")),
		Password:      os.Getenv("NAVIDROME_PASSWORD"),
		PlaylistSize:  playlistSize,
		AlbumPageSize: albumPageSize,
		RunTimeout:    runTimeout,
		ClientName:    getString("SUBSONIC_CLIENT_NAME", defaultClientName),
		APIVersion:    getString("SUBSONIC_API_VERSION", defaultAPIVersion),
		StateFile:     resolveStateFile(),
		EnableState:   enableState,
		MinBackfill:   minBackfill,
		Weights: Weights{
			PlayCount: playCountWeight,
			Recency:   recencyWeight,
			Freshness: freshnessWeight,
			DecayDays: decayDays,
		},
	}

	if cfg.BaseURL == "" || cfg.Username == "" || cfg.Password == "" {
		return Config{}, errors.New("NAVIDROME_URL, NAVIDROME_USER, and NAVIDROME_PASSWORD are required")
	}

	if cfg.PlaylistSize <= 0 {
		return Config{}, fmt.Errorf("PLAYLIST_SIZE must be positive, got %d", cfg.PlaylistSize)
	}

	if cfg.AlbumPageSize <= 0 {
		return Config{}, fmt.Errorf("ALBUM_PAGE_SIZE must be positive, got %d", cfg.AlbumPageSize)
	}

	if cfg.Weights.DecayDays <= 0 {
		return Config{}, fmt.Errorf("SCORE_DECAY_DAYS must be positive, got %.2f", cfg.Weights.DecayDays)
	}
	if cfg.Weights.PlayCount < 0 || cfg.Weights.Recency < 0 || cfg.Weights.Freshness < 0 {
		return Config{}, errors.New("score weights must be non-negative")
	}
	if cfg.RunTimeout <= 0 {
		return Config{}, fmt.Errorf("RUN_TIMEOUT must be positive, got %s", cfg.RunTimeout)
	}

	if cfg.MinBackfill < 0 {
		return Config{}, fmt.Errorf("MIN_CANDIDATE_BACKFILL must be non-negative, got %d", cfg.MinBackfill)
	}

	return cfg, nil
}

func resolveStateFile() string {
	stateFile := strings.TrimSpace(os.Getenv("STATE_FILE"))
	if stateFile != "" {
		return stateFile
	}

	stateDir := strings.TrimSpace(os.Getenv("STATE_DIR"))
	if stateDir == "" {
		stateDir = defaultStateDir
	}

	return filepath.Join(stateDir, defaultStateFileName)
}

func getString(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	return value
}

func getInt(key string, fallback int) (int, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer, got %q: %w", key, value, err)
	}

	return parsed, nil
}

func getFloat(key string, fallback float64) (float64, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}

	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be a number, got %q: %w", key, value, err)
	}
	if math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return 0, fmt.Errorf("%s must be finite, got %q", key, value)
	}

	return parsed, nil
}

func getBool(key string, fallback bool) (bool, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}

	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean, got %q: %w", key, value, err)
	}

	return parsed, nil
}

func getDuration(key string, fallback time.Duration) (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}

	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be a duration, got %q: %w", key, value, err)
	}

	return parsed, nil
}
