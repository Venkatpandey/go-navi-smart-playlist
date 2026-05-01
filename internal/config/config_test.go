package config

import "testing"

func TestLoadLyricsDefaults(t *testing.T) {
	t.Setenv("NAVIDROME_URL", "http://navidrome:4533/")
	t.Setenv("NAVIDROME_USER", "user")
	t.Setenv("NAVIDROME_PASSWORD", "password")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.BaseURL != "http://navidrome:4533" {
		t.Fatalf("expected trimmed base URL, got %q", cfg.BaseURL)
	}
	if cfg.Lyrics.Enabled {
		t.Fatalf("lyrics should default off")
	}
	if cfg.Lyrics.FetchWorkers != 10 {
		t.Fatalf("expected 10 fetch workers, got %d", cfg.Lyrics.FetchWorkers)
	}
	if cfg.Lyrics.WriteWorkers != 100 {
		t.Fatalf("expected 100 write workers, got %d", cfg.Lyrics.WriteWorkers)
	}
	if !cfg.Lyrics.TriggerScan {
		t.Fatalf("trigger scan should default true")
	}
}

func TestLoadRejectsInvalidLyricsConfig(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
	}{
		{
			name: "fetch workers",
			env:  map[string]string{"LYRICS_FETCH_WORKERS": "0"},
		},
		{
			name: "write workers",
			env:  map[string]string{"LYRICS_WRITE_WORKERS": "0"},
		},
		{
			name: "path prefix pair",
			env:  map[string]string{"LYRICS_PATH_PREFIX_FROM": "/host"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("NAVIDROME_URL", "http://navidrome:4533")
			t.Setenv("NAVIDROME_USER", "user")
			t.Setenv("NAVIDROME_PASSWORD", "password")
			for key, value := range tt.env {
				t.Setenv(key, value)
			}

			if _, err := Load(); err == nil {
				t.Fatalf("expected error")
			}
		})
	}
}
