package config

import (
	"errors"
	"fmt"
	"os"
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
	defaultFetchWorkers  = 10
	defaultWriteWorkers  = 100
)

type Config struct {
	BaseURL       string
	Username      string
	Password      string
	PlaylistSize  int
	AlbumPageSize int
	DryRun        bool
	Weights       Weights
	RunTimeout    time.Duration
	ClientName    string
	APIVersion    string
	StateFile     string
	EnableState   bool
	MinBackfill   int
	Lyrics        LyricsConfig
}

type LyricsConfig struct {
	Enabled      bool
	Overwrite    bool
	FetchWorkers int
	WriteWorkers int
	PathFrom     string
	PathTo       string
	TriggerScan  bool
}

type Weights struct {
	PlayCount float64
	Recency   float64
	Freshness float64
	DecayDays float64
}

func Load() (Config, error) {
	cfg := Config{
		BaseURL:       strings.TrimRight(strings.TrimSpace(os.Getenv("NAVIDROME_URL")), "/"),
		Username:      strings.TrimSpace(os.Getenv("NAVIDROME_USER")),
		Password:      os.Getenv("NAVIDROME_PASSWORD"),
		PlaylistSize:  getInt("PLAYLIST_SIZE", defaultPlaylistSize),
		AlbumPageSize: getInt("ALBUM_PAGE_SIZE", defaultAlbumPageSize),
		DryRun:        getBool("DRY_RUN", false),
		RunTimeout:    getDuration("RUN_TIMEOUT", defaultRunTimeout),
		ClientName:    getString("SUBSONIC_CLIENT_NAME", defaultClientName),
		APIVersion:    getString("SUBSONIC_API_VERSION", defaultAPIVersion),
		StateFile:     resolveStateFile(),
		EnableState:   getBool("ENABLE_STATE_CACHE", true),
		MinBackfill:   getInt("MIN_CANDIDATE_BACKFILL", defaultBackfillSize),
		Lyrics: LyricsConfig{
			Enabled:      getBool("ENABLE_LYRICS", false),
			Overwrite:    getBool("LYRICS_OVERWRITE", false),
			FetchWorkers: getInt("LYRICS_FETCH_WORKERS", defaultFetchWorkers),
			WriteWorkers: getInt("LYRICS_WRITE_WORKERS", defaultWriteWorkers),
			PathFrom:     strings.TrimSpace(os.Getenv("LYRICS_PATH_PREFIX_FROM")),
			PathTo:       strings.TrimSpace(os.Getenv("LYRICS_PATH_PREFIX_TO")),
			TriggerScan:  getBool("LYRICS_TRIGGER_SCAN", true),
		},
		Weights: Weights{
			PlayCount: getFloat("SCORE_WEIGHT_PLAYCOUNT", 1.0),
			Recency:   getFloat("SCORE_WEIGHT_RECENCY", 2.0),
			Freshness: getFloat("SCORE_WEIGHT_FRESHNESS", 1.5),
			DecayDays: getFloat("SCORE_DECAY_DAYS", defaultDecayDays),
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

	if cfg.MinBackfill < 0 {
		return Config{}, fmt.Errorf("MIN_CANDIDATE_BACKFILL must be non-negative, got %d", cfg.MinBackfill)
	}

	if cfg.Lyrics.FetchWorkers <= 0 {
		return Config{}, fmt.Errorf("LYRICS_FETCH_WORKERS must be positive, got %d", cfg.Lyrics.FetchWorkers)
	}

	if cfg.Lyrics.WriteWorkers <= 0 {
		return Config{}, fmt.Errorf("LYRICS_WRITE_WORKERS must be positive, got %d", cfg.Lyrics.WriteWorkers)
	}

	if (cfg.Lyrics.PathFrom == "") != (cfg.Lyrics.PathTo == "") {
		return Config{}, errors.New("LYRICS_PATH_PREFIX_FROM and LYRICS_PATH_PREFIX_TO must be set together")
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

	return stateDir + "/" + defaultStateFileName
}

func getString(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	return value
}

func getInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}

	return parsed
}

func getFloat(key string, fallback float64) float64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fallback
	}

	return parsed
}

func getBool(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}

	return parsed
}

func getDuration(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}

	return parsed
}
