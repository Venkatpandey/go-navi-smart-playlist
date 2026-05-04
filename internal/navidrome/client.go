package navidrome

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"go-navi-smart-playlist/internal/config"
)

type Client struct {
	baseURL    string
	username   string
	password   string
	clientName string
	apiVersion string
	httpClient *http.Client
	logger     *log.Logger
}

type Album struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Title string `json:"title"`
}

type TrackPayload struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Artist     string `json:"artist"`
	Album      string `json:"album"`
	PlayCount  int    `json:"playCount"`
	Played     string `json:"played"`
	Created    string `json:"created"`
	Starred    string `json:"starred"`
	UserRating int    `json:"userRating"`
}

type Playlist struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Owner     string `json:"owner"`
	Public    bool   `json:"public"`
	SongCount int    `json:"songCount"`
}

type albumList2Response struct {
	SubsonicResponse struct {
		Status string `json:"status"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
		AlbumList2 struct {
			Albums []Album `json:"album"`
		} `json:"albumList2"`
	} `json:"subsonic-response"`
}

type albumResponse struct {
	SubsonicResponse struct {
		Status string `json:"status"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
		Album struct {
			ID    string         `json:"id"`
			Name  string         `json:"name"`
			Title string         `json:"title"`
			Songs []TrackPayload `json:"song"`
		} `json:"album"`
	} `json:"subsonic-response"`
}

type playlistsResponse struct {
	SubsonicResponse struct {
		Status string `json:"status"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
		Playlists struct {
			Items []Playlist `json:"playlist"`
		} `json:"playlists"`
	} `json:"subsonic-response"`
}

type playlistResponse struct {
	SubsonicResponse struct {
		Status string `json:"status"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
		Playlist struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Entries []struct {
				ID string `json:"id"`
			} `json:"entry"`
		} `json:"playlist"`
	} `json:"subsonic-response"`
}

type mutationResponse struct {
	SubsonicResponse struct {
		Status string `json:"status"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	} `json:"subsonic-response"`
}

func NewClient(cfg config.Config, logger *log.Logger) *Client {
	return &Client{
		baseURL:    cfg.BaseURL,
		username:   cfg.Username,
		password:   cfg.Password,
		clientName: cfg.ClientName,
		apiVersion: cfg.APIVersion,
		httpClient: &http.Client{Timeout: cfg.RunTimeout},
		logger:     logger,
	}
}

func NewClientWithCredentials(cfg config.Config, username, password string, logger *log.Logger) *Client {
	return &Client{
		baseURL:    cfg.BaseURL,
		username:   username,
		password:   password,
		clientName: cfg.ClientName,
		apiVersion: cfg.APIVersion,
		httpClient: &http.Client{Timeout: cfg.RunTimeout},
		logger:     logger,
	}
}

func (c *Client) GetAlbumList2(ctx context.Context, size, offset int) ([]Album, error) {
	query := url.Values{}
	query.Set("type", "alphabeticalByName")
	query.Set("size", strconv.Itoa(size))
	query.Set("offset", strconv.Itoa(offset))

	var payload albumList2Response
	if err := c.get(ctx, "getAlbumList2", query, &payload); err != nil {
		return nil, err
	}

	return payload.SubsonicResponse.AlbumList2.Albums, nil
}

func (c *Client) GetAlbum(ctx context.Context, albumID string) ([]TrackPayload, error) {
	query := url.Values{}
	query.Set("id", albumID)

	var payload albumResponse
	if err := c.get(ctx, "getAlbum", query, &payload); err != nil {
		return nil, err
	}

	return payload.SubsonicResponse.Album.Songs, nil
}

func (c *Client) GetPlaylists(ctx context.Context) ([]Playlist, error) {
	var payload playlistsResponse
	if err := c.get(ctx, "getPlaylists", nil, &payload); err != nil {
		return nil, err
	}

	return payload.SubsonicResponse.Playlists.Items, nil
}

func (c *Client) GetPlaylistSongCount(ctx context.Context, playlistID string) (int, error) {
	query := url.Values{}
	query.Set("id", playlistID)

	var payload playlistResponse
	if err := c.get(ctx, "getPlaylist", query, &payload); err != nil {
		return 0, err
	}

	return len(payload.SubsonicResponse.Playlist.Entries), nil
}

func (c *Client) CreatePlaylist(ctx context.Context, name string, songIDs []string) error {
	query := url.Values{}
	query.Set("name", name)
	for _, songID := range songIDs {
		query.Add("songId", songID)
	}

	return c.post(ctx, "createPlaylist", query)
}

func (c *Client) UpdatePlaylist(ctx context.Context, playlistID string, removeCount int, songIDs []string) error {
	query := url.Values{}
	query.Set("playlistId", playlistID)
	for index := removeCount - 1; index >= 0; index-- {
		query.Add("songIndexToRemove", strconv.Itoa(index))
	}
	for _, songID := range songIDs {
		query.Add("songIdToAdd", songID)
	}

	return c.post(ctx, "updatePlaylist", query)
}

func (c *Client) get(ctx context.Context, endpoint string, query url.Values, target any) error {
	return c.do(ctx, http.MethodGet, endpoint, query, target)
}

func (c *Client) post(ctx context.Context, endpoint string, query url.Values) error {
	var payload mutationResponse
	return c.do(ctx, http.MethodPost, endpoint, query, &payload)
}

func (c *Client) do(ctx context.Context, method, endpoint string, query url.Values, target any) error {
	if query == nil {
		query = url.Values{}
	}

	for key, value := range c.authQuery() {
		for _, entry := range value {
			query.Add(key, entry)
		}
	}

	endpointURL := fmt.Sprintf("%s/rest/%s.view?%s", c.baseURL, endpoint, query.Encode())
	req, err := http.NewRequestWithContext(ctx, method, endpointURL, nil)
	if err != nil {
		return fmt.Errorf("create %s request: %w", endpoint, err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%s request failed: %w", endpoint, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8*1024))
		return fmt.Errorf("%s unexpected status %d: %s", endpoint, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return fmt.Errorf("decode %s response: %w", endpoint, err)
	}

	if err := extractError(target); err != nil {
		return fmt.Errorf("%s api error: %w", endpoint, err)
	}

	return nil
}

func (c *Client) authQuery() url.Values {
	salt := randomSalt(8)
	tokenHash := md5.Sum([]byte(c.password + salt))

	query := url.Values{}
	query.Set("u", c.username)
	query.Set("t", hex.EncodeToString(tokenHash[:]))
	query.Set("s", salt)
	query.Set("v", c.apiVersion)
	query.Set("c", c.clientName)
	query.Set("f", "json")

	return query
}

func randomSalt(length int) string {
	raw := make([]byte, length)
	if _, err := rand.Read(raw); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}

	return hex.EncodeToString(raw)[:length]
}

func extractError(target any) error {
	switch payload := target.(type) {
	case *albumList2Response:
		return responseError(payload.SubsonicResponse.Error)
	case *albumResponse:
		return responseError(payload.SubsonicResponse.Error)
	case *playlistsResponse:
		return responseError(payload.SubsonicResponse.Error)
	case *playlistResponse:
		return responseError(payload.SubsonicResponse.Error)
	case *mutationResponse:
		return responseError(payload.SubsonicResponse.Error)
	default:
		return nil
	}
}

func responseError(errPayload *struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}) error {
	if errPayload == nil {
		return nil
	}

	return fmt.Errorf("code=%d message=%s", errPayload.Code, errPayload.Message)
}
