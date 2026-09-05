package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	setBaseEnvironment(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.PlaylistSize != defaultPlaylistSize {
		t.Fatalf("expected playlist size %d, got %d", defaultPlaylistSize, cfg.PlaylistSize)
	}
	if cfg.RunTimeout != defaultRunTimeout {
		t.Fatalf("expected timeout %s, got %s", defaultRunTimeout, cfg.RunTimeout)
	}
}

func TestLoadRejectsInvalidEnvironmentValues(t *testing.T) {
	tests := []struct {
		key   string
		value string
		want  string
	}{
		{key: "PLAYLIST_SIZE", value: "many", want: "must be an integer"},
		{key: "ENABLE_STATE_CACHE", value: "sometimes", want: "must be a boolean"},
		{key: "RUN_TIMEOUT", value: "later", want: "must be a duration"},
		{key: "RUN_TIMEOUT", value: "0s", want: "must be positive"},
		{key: "SCORE_WEIGHT_PLAYCOUNT", value: "NaN", want: "must be finite"},
		{key: "SCORE_WEIGHT_RECENCY", value: "-1", want: "must be non-negative"},
	}

	for _, test := range tests {
		t.Run(test.key+"="+test.value, func(t *testing.T) {
			setBaseEnvironment(t)
			t.Setenv(test.key, test.value)

			_, err := Load()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected error containing %q, got %v", test.want, err)
			}
		})
	}
}

func TestLoadAcceptsExplicitValues(t *testing.T) {
	setBaseEnvironment(t)
	t.Setenv("PLAYLIST_SIZE", "25")
	t.Setenv("ENABLE_STATE_CACHE", "false")
	t.Setenv("RUN_TIMEOUT", "2m")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.PlaylistSize != 25 || cfg.EnableState || cfg.RunTimeout != 2*time.Minute {
		t.Fatalf("unexpected parsed config: %+v", cfg)
	}
}

func setBaseEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv("NAVIDROME_URL", "http://navidrome:4533")
	t.Setenv("NAVIDROME_USER", "listener")
	t.Setenv("NAVIDROME_PASSWORD", "secret")
	for _, key := range []string{
		"PLAYLIST_SIZE",
		"ALBUM_PAGE_SIZE",
		"ENABLE_STATE_CACHE",
		"RUN_TIMEOUT",
		"MIN_CANDIDATE_BACKFILL",
		"SCORE_WEIGHT_PLAYCOUNT",
		"SCORE_WEIGHT_RECENCY",
		"SCORE_WEIGHT_FRESHNESS",
		"SCORE_DECAY_DAYS",
		"STATE_FILE",
		"STATE_DIR",
	} {
		t.Setenv(key, "")
	}
}
