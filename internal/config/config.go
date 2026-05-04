package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	BaseURL             string
	Username            string
	Password            string
	AdminUsername       string
	AdminPassword       string
	PlaylistSize        int
	AlbumPageSize       int
	DryRun              bool
	Weights             Weights
	RunTimeout          time.Duration
	ClientName          string
	APIVersion          string
	StateFile           string
	StateDir            string
	EnableState         bool
	MinBackfill         int
	MultiUserEnabled    bool
	MultiUserConfigFile string
}

type Weights struct {
	PlayCount float64
	Recency   float64
	Freshness float64
	DecayDays float64
}

type MultiUserFile struct {
	Users []UserCredential `json:"users"`
}

type UserCredential struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Enabled  *bool  `json:"enabled,omitempty"`
}

func Load() (Config, error) {
	stateFileEnv := strings.TrimSpace(os.Getenv("STATE_FILE"))
	stateDirEnv := strings.TrimSpace(os.Getenv("STATE_DIR"))

	cfg := Config{
		BaseURL:             strings.TrimRight(strings.TrimSpace(os.Getenv("NAVIDROME_URL")), "/"),
		Username:            strings.TrimSpace(os.Getenv("NAVIDROME_USER")),
		Password:            os.Getenv("NAVIDROME_PASSWORD"),
		AdminUsername:       strings.TrimSpace(os.Getenv("NAVIDROME_ADMIN_USER")),
		AdminPassword:       os.Getenv("NAVIDROME_ADMIN_PASSWORD"),
		PlaylistSize:        getInt("PLAYLIST_SIZE", defaultPlaylistSize),
		AlbumPageSize:       getInt("ALBUM_PAGE_SIZE", defaultAlbumPageSize),
		DryRun:              getBool("DRY_RUN", false),
		RunTimeout:          getDuration("RUN_TIMEOUT", defaultRunTimeout),
		ClientName:          getString("SUBSONIC_CLIENT_NAME", defaultClientName),
		APIVersion:          getString("SUBSONIC_API_VERSION", defaultAPIVersion),
		StateFile:           resolveStateFile(stateFileEnv, stateDirEnv),
		StateDir:            stateDirEnv,
		EnableState:         getBool("ENABLE_STATE_CACHE", true),
		MinBackfill:         getInt("MIN_CANDIDATE_BACKFILL", defaultBackfillSize),
		MultiUserEnabled:    getBool("MULTI_USER_ENABLED", false),
		MultiUserConfigFile: strings.TrimSpace(os.Getenv("MULTI_USER_CONFIG_FILE")),
		Weights: Weights{
			PlayCount: getFloat("SCORE_WEIGHT_PLAYCOUNT", 1.0),
			Recency:   getFloat("SCORE_WEIGHT_RECENCY", 2.0),
			Freshness: getFloat("SCORE_WEIGHT_FRESHNESS", 1.5),
			DecayDays: getFloat("SCORE_DECAY_DAYS", defaultDecayDays),
		},
	}

	if cfg.BaseURL == "" {
		return Config{}, errors.New("NAVIDROME_URL is required")
	}

	if cfg.MultiUserEnabled {
		if cfg.AdminUsername == "" || cfg.AdminPassword == "" {
			return Config{}, errors.New("NAVIDROME_ADMIN_USER and NAVIDROME_ADMIN_PASSWORD are required when MULTI_USER_ENABLED=true")
		}
		if cfg.MultiUserConfigFile == "" {
			return Config{}, errors.New("MULTI_USER_CONFIG_FILE is required when MULTI_USER_ENABLED=true")
		}
		if cfg.StateDir == "" {
			return Config{}, errors.New("STATE_DIR is required when MULTI_USER_ENABLED=true")
		}
		if stateFileEnv != "" {
			return Config{}, errors.New("STATE_FILE is not supported when MULTI_USER_ENABLED=true; use STATE_DIR")
		}
	} else if cfg.Username == "" || cfg.Password == "" {
		return Config{}, errors.New("NAVIDROME_USER and NAVIDROME_PASSWORD are required")
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

	return cfg, nil
}

func (c Config) StateFileForUser(username string) string {
	if c.MultiUserEnabled {
		return filepath.Join(c.StateDir, username, defaultStateFileName)
	}

	return c.StateFile
}

func LoadUserCredentials(path string) ([]UserCredential, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open multi-user config file: %w", err)
	}
	defer file.Close()

	var payload MultiUserFile
	if err := json.NewDecoder(file).Decode(&payload); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, errors.New("multi-user config file is empty")
		}

		return nil, fmt.Errorf("decode multi-user config file: %w", err)
	}

	if len(payload.Users) == 0 {
		return nil, errors.New("multi-user config file must include at least one user")
	}

	seen := make(map[string]struct{}, len(payload.Users))
	users := make([]UserCredential, 0, len(payload.Users))
	for index, user := range payload.Users {
		user.Username = strings.TrimSpace(user.Username)
		if user.Username == "" {
			return nil, fmt.Errorf("multi-user config entry %d is missing username", index)
		}
		if user.Password == "" {
			return nil, fmt.Errorf("multi-user config entry %q is missing password", user.Username)
		}

		key := strings.ToLower(user.Username)
		if _, ok := seen[key]; ok {
			return nil, fmt.Errorf("duplicate username in multi-user config: %s", user.Username)
		}
		seen[key] = struct{}{}
		users = append(users, user)
	}

	return users, nil
}

func (u UserCredential) IsEnabled() bool {
	return u.Enabled == nil || *u.Enabled
}

func resolveStateFile(stateFile, stateDir string) string {
	if stateFile != "" {
		return stateFile
	}

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
