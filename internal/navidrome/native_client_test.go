package navidrome

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go-navi-smart-playlist/internal/config"
)

func TestNativeClientLogin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/login" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method %s", r.Method)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("unexpected content type %q", got)
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if string(body) != `{"username":"admin","password":"secret"}` {
			t.Fatalf("unexpected body %s", string(body))
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"token":"jwt-token"}`))
	}))
	defer server.Close()

	client := NewNativeClient(config.Config{
		BaseURL:    server.URL,
		RunTimeout: time.Second,
	}, log.New(io.Discard, "", 0))

	token, err := client.Login(context.Background(), "admin", "secret")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if token != "jwt-token" {
		t.Fatalf("expected jwt-token, got %q", token)
	}
}

func TestNativeClientDiscoverUsers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/user" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if got := r.Header.Get("X-ND-Authorization"); got != "Bearer jwt-token" {
			t.Fatalf("unexpected auth header %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"items": [
				{"userName":"alice","isAdmin":true,"enabled":true,"ignored":"value"},
				{"username":"bob","isAdmin":false,"disabled":true}
			]
		}`))
	}))
	defer server.Close()

	client := NewNativeClient(config.Config{
		BaseURL:    server.URL,
		RunTimeout: time.Second,
	}, log.New(io.Discard, "", 0))

	users, err := client.DiscoverUsers(context.Background(), "jwt-token")
	if err != nil {
		t.Fatalf("discover users: %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("expected 2 users, got %d", len(users))
	}
	if users[0].Username != "alice" || !users[0].IsAdmin || users[0].Enabled == nil || !*users[0].Enabled {
		t.Fatalf("unexpected first user %+v", users[0])
	}
	if users[1].Username != "bob" || users[1].Enabled == nil || *users[1].Enabled {
		t.Fatalf("unexpected second user %+v", users[1])
	}
}

func TestDecodeDiscoveredUsersSupportsArrayShape(t *testing.T) {
	users, err := decodeDiscoveredUsers([]byte(`[{"username":"alice","extra":"x"}]`))
	if err != nil {
		t.Fatalf("decode users: %v", err)
	}
	if len(users) != 1 || users[0].Username != "alice" {
		t.Fatalf("unexpected users %+v", users)
	}
}

func TestNativeClientLoginMissingToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"token":"   "}`))
	}))
	defer server.Close()

	client := NewNativeClient(config.Config{
		BaseURL:    server.URL,
		RunTimeout: time.Second,
	}, log.New(io.Discard, "", 0))

	_, err := client.Login(context.Background(), "admin", "secret")
	if err == nil || !strings.Contains(err.Error(), "missing token") {
		t.Fatalf("expected missing token error, got %v", err)
	}
}
