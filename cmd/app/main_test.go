package main

import (
	"bytes"
	"context"
	"errors"
	"log"
	"reflect"
	"strings"
	"testing"

	"go-navi-smart-playlist/internal/config"
	"go-navi-smart-playlist/internal/navidrome"
)

type fakeDiscoverer struct {
	token string
	users []navidrome.DiscoveredUser
	err   error
}

func (f fakeDiscoverer) Login(ctx context.Context, username, password string) (string, error) {
	if f.err != nil {
		return "", f.err
	}

	return f.token, nil
}

func (f fakeDiscoverer) DiscoverUsers(ctx context.Context, token string) ([]navidrome.DiscoveredUser, error) {
	if f.err != nil {
		return nil, f.err
	}

	return f.users, nil
}

func TestRunMultiUserSerialBestEffort(t *testing.T) {
	cfg := config.Config{
		MultiUserEnabled:    true,
		AdminUsername:       "admin",
		AdminPassword:       "secret",
		MultiUserConfigFile: "/tmp/users.json",
		StateDir:            "/state",
	}

	users := []config.UserCredential{
		{Username: "alice", Password: "alice-pass"},
		{Username: "bob", Password: "bob-pass"},
		{Username: "ghost", Password: "ghost-pass"},
	}

	var order []string
	var stateFiles []string
	err := runMultiUser(
		context.Background(),
		cfg,
		log.New(&bytes.Buffer{}, "", 0),
		fakeDiscoverer{
			token: "jwt-token",
			users: []navidrome.DiscoveredUser{
				{Username: "alice"},
				{Username: "bob"},
				{Username: "charlie"},
			},
		},
		func(path string) ([]config.UserCredential, error) {
			if path != "/tmp/users.json" {
				t.Fatalf("unexpected config path %q", path)
			}

			return users, nil
		},
		func(ctx context.Context, cfg config.Config, user config.UserCredential, stateFile string, logger *log.Logger) error {
			order = append(order, user.Username)
			stateFiles = append(stateFiles, stateFile)
			if user.Username == "bob" {
				return errors.New("boom")
			}

			return nil
		},
	)
	if err == nil || !strings.Contains(err.Error(), "failed=2") {
		t.Fatalf("expected partial failure error, got %v", err)
	}

	if !reflect.DeepEqual(order, []string{"alice", "bob"}) {
		t.Fatalf("unexpected order %v", order)
	}
	if !reflect.DeepEqual(stateFiles, []string{"/state/alice/state.json", "/state/bob/state.json"}) {
		t.Fatalf("unexpected state files %v", stateFiles)
	}
}

func TestRunMultiUserIgnoresDisabledCredentialsAndDiscoveredDisabledUsers(t *testing.T) {
	cfg := config.Config{
		MultiUserEnabled:    true,
		AdminUsername:       "admin",
		AdminPassword:       "secret",
		MultiUserConfigFile: "/tmp/users.json",
		StateDir:            "/state",
	}

	disabled := false
	enabled := true
	var ran []string
	err := runMultiUser(
		context.Background(),
		cfg,
		log.New(&bytes.Buffer{}, "", 0),
		fakeDiscoverer{
			token: "jwt-token",
			users: []navidrome.DiscoveredUser{
				{Username: "alice", Enabled: &enabled},
				{Username: "bob", Enabled: &disabled},
			},
		},
		func(path string) ([]config.UserCredential, error) {
			return []config.UserCredential{
				{Username: "alice", Password: "alice-pass"},
				{Username: "bob", Password: "bob-pass"},
			}, nil
		},
		func(ctx context.Context, cfg config.Config, user config.UserCredential, stateFile string, logger *log.Logger) error {
			ran = append(ran, user.Username)
			return nil
		},
	)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if !reflect.DeepEqual(ran, []string{"alice"}) {
		t.Fatalf("unexpected users run %v", ran)
	}
}
