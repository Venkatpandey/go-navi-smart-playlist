package lyrics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"go-navi-smart-playlist/internal/model"
)

const defaultLRCLIBBaseURL = "https://lrclib.net"

var ErrNotFound = errors.New("lyrics not found")

type LRCLIBProvider struct {
	baseURL    string
	httpClient *http.Client
}

type lrclibResponse struct {
	Instrumental bool   `json:"instrumental"`
	PlainLyrics  string `json:"plainLyrics"`
	SyncedLyrics string `json:"syncedLyrics"`
}

func NewLRCLIBProvider() *LRCLIBProvider {
	return NewLRCLIBProviderWithClient(defaultLRCLIBBaseURL, &http.Client{Timeout: 30 * time.Second})
}

func NewLRCLIBProviderWithClient(baseURL string, httpClient *http.Client) *LRCLIBProvider {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = defaultLRCLIBBaseURL
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}

	return &LRCLIBProvider{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: httpClient,
	}
}

func (p *LRCLIBProvider) Find(ctx context.Context, track model.Track) (Result, error) {
	query := url.Values{}
	query.Set("track_name", track.Title)
	query.Set("artist_name", track.Artist)
	if strings.TrimSpace(track.Album) != "" {
		query.Set("album_name", track.Album)
	}
	if track.Duration > 0 {
		query.Set("duration", strconv.Itoa(track.Duration))
	}

	endpoint := p.baseURL + "/api/get?" + query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Result{}, fmt.Errorf("create LRCLIB request: %w", err)
	}
	req.Header.Set("User-Agent", "go-navi-smart-playlist")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("LRCLIB request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return Result{}, ErrNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Result{}, fmt.Errorf("LRCLIB unexpected status %d", resp.StatusCode)
	}

	var payload lrclibResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return Result{}, fmt.Errorf("decode LRCLIB response: %w", err)
	}
	if payload.Instrumental {
		return Result{}, ErrNotFound
	}

	return Result{
		Synced: payload.SyncedLyrics,
		Plain:  payload.PlainLyrics,
	}, nil
}
