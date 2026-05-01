package lyrics

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"go-navi-smart-playlist/internal/model"
)

func TestLRCLIBProviderFindsLyrics(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/get" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		if r.URL.Query().Get("track_name") != "Song" {
			t.Fatalf("missing track query")
		}
		if r.URL.Query().Get("duration") != "123" {
			t.Fatalf("missing duration query")
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"plainLyrics":"plain","syncedLyrics":"[00:01.00]line"}`))
	}))
	defer server.Close()

	provider := NewLRCLIBProviderWithClient(server.URL, server.Client())
	result, err := provider.Find(context.Background(), model.Track{
		Title:    "Song",
		Artist:   "Artist",
		Album:    "Album",
		Duration: 123,
	})
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if result.Synced != "[00:01.00]line" || result.Plain != "plain" {
		t.Fatalf("unexpected result %+v", result)
	}
}

func TestLRCLIBProviderMapsNotFoundAndInstrumental(t *testing.T) {
	for name, body := range map[string]string{
		"not-found":    "",
		"instrumental": `{"instrumental":true}`,
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if name == "not-found" {
					http.NotFound(w, r)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(body))
			}))
			defer server.Close()

			provider := NewLRCLIBProviderWithClient(server.URL, server.Client())
			_, err := provider.Find(context.Background(), model.Track{Title: "Song", Artist: "Artist"})
			if !errors.Is(err, ErrNotFound) {
				t.Fatalf("expected ErrNotFound, got %v", err)
			}
		})
	}
}
