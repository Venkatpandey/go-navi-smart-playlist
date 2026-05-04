package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSingleUserMode(t *testing.T) {
	t.Setenv("NAVIDROME_URL", "http://navidrome:4533")
	t.Setenv("NAVIDROME_USER", "alice")
	t.Setenv("NAVIDROME_PASSWORD", "secret")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.MultiUserEnabled {
		t.Fatalf("expected single-user mode")
	}
	if cfg.Username != "alice" {
		t.Fatalf("expected username alice, got %q", cfg.Username)
	}
	if cfg.StateFile != "/tmp/go-smart-playlist/state.json" {
		t.Fatalf("unexpected default state file %q", cfg.StateFile)
	}
}

func TestLoadMultiUserModeRequiresAdminCreds(t *testing.T) {
	t.Setenv("NAVIDROME_URL", "http://navidrome:4533")
	t.Setenv("MULTI_USER_ENABLED", "true")
	t.Setenv("MULTI_USER_CONFIG_FILE", "/tmp/users.json")
	t.Setenv("STATE_DIR", "/tmp/state")

	_, err := Load()
	if err == nil || err.Error() != "NAVIDROME_ADMIN_USER and NAVIDROME_ADMIN_PASSWORD are required when MULTI_USER_ENABLED=true" {
		t.Fatalf("expected admin creds error, got %v", err)
	}
}

func TestLoadMultiUserModeRequiresStateDir(t *testing.T) {
	t.Setenv("NAVIDROME_URL", "http://navidrome:4533")
	t.Setenv("MULTI_USER_ENABLED", "true")
	t.Setenv("NAVIDROME_ADMIN_USER", "admin")
	t.Setenv("NAVIDROME_ADMIN_PASSWORD", "secret")
	t.Setenv("MULTI_USER_CONFIG_FILE", "/tmp/users.json")

	_, err := Load()
	if err == nil || err.Error() != "STATE_DIR is required when MULTI_USER_ENABLED=true" {
		t.Fatalf("expected state dir error, got %v", err)
	}
}

func TestLoadMultiUserModeRejectsStateFile(t *testing.T) {
	t.Setenv("NAVIDROME_URL", "http://navidrome:4533")
	t.Setenv("MULTI_USER_ENABLED", "true")
	t.Setenv("NAVIDROME_ADMIN_USER", "admin")
	t.Setenv("NAVIDROME_ADMIN_PASSWORD", "secret")
	t.Setenv("MULTI_USER_CONFIG_FILE", "/tmp/users.json")
	t.Setenv("STATE_DIR", "/tmp/state")
	t.Setenv("STATE_FILE", "/tmp/state.json")

	_, err := Load()
	if err == nil || err.Error() != "STATE_FILE is not supported when MULTI_USER_ENABLED=true; use STATE_DIR" {
		t.Fatalf("expected state file error, got %v", err)
	}
}

func TestLoadUserCredentialsRejectsDuplicates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "users.json")
	content := `{"users":[{"username":"alice","password":"one"},{"username":"Alice","password":"two"}]}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write users.json: %v", err)
	}

	_, err := LoadUserCredentials(path)
	if err == nil || err.Error() != "duplicate username in multi-user config: Alice" {
		t.Fatalf("expected duplicate error, got %v", err)
	}
}
