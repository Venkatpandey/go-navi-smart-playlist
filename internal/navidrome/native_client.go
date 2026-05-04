package navidrome

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"go-navi-smart-playlist/internal/config"
)

type NativeClient struct {
	baseURL    string
	httpClient *http.Client
	logger     *log.Logger
}

type DiscoveredUser struct {
	Username string
	IsAdmin  bool
	Enabled  *bool
}

func NewNativeClient(cfg config.Config, logger *log.Logger) *NativeClient {
	return &NativeClient{
		baseURL:    cfg.BaseURL,
		httpClient: &http.Client{Timeout: cfg.RunTimeout},
		logger:     logger,
	}
}

func (c *NativeClient) Login(ctx context.Context, username, password string) (string, error) {
	body := bytes.NewBufferString(fmt.Sprintf(`{"username":%q,"password":%q}`, username, password))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/auth/login", body)
	if err != nil {
		return "", fmt.Errorf("create auth/login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("auth/login request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		payload, _ := io.ReadAll(io.LimitReader(resp.Body, 8*1024))
		return "", fmt.Errorf("auth/login unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}

	var decoded struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return "", fmt.Errorf("decode auth/login response: %w", err)
	}
	if strings.TrimSpace(decoded.Token) == "" {
		return "", fmt.Errorf("auth/login response missing token")
	}

	return decoded.Token, nil
}

func (c *NativeClient) DiscoverUsers(ctx context.Context, token string) ([]DiscoveredUser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/user", nil)
	if err != nil {
		return nil, fmt.Errorf("create /api/user request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-ND-Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("/api/user request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		payload, _ := io.ReadAll(io.LimitReader(resp.Body, 8*1024))
		return nil, fmt.Errorf("/api/user unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}

	payload, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read /api/user response: %w", err)
	}

	users, err := decodeDiscoveredUsers(payload)
	if err != nil {
		return nil, fmt.Errorf("decode /api/user response: %w", err)
	}

	return users, nil
}

func decodeDiscoveredUsers(payload []byte) ([]DiscoveredUser, error) {
	var decoded any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return nil, err
	}

	items, err := userItems(decoded)
	if err != nil {
		return nil, err
	}

	users := make([]DiscoveredUser, 0, len(items))
	for _, raw := range items {
		object, ok := raw.(map[string]any)
		if !ok {
			continue
		}

		username := firstString(object, "username", "userName", "user_name")
		if strings.TrimSpace(username) == "" {
			continue
		}

		users = append(users, DiscoveredUser{
			Username: strings.TrimSpace(username),
			IsAdmin:  firstBool(object, "isAdmin", "is_admin"),
			Enabled:  firstOptionalBool(object, "enabled", "isEnabled", "disabled", "isDisabled"),
		})
	}

	return users, nil
}

func userItems(decoded any) ([]any, error) {
	switch payload := decoded.(type) {
	case []any:
		return payload, nil
	case map[string]any:
		for _, key := range []string{"items", "users", "data"} {
			if items, ok := payload[key].([]any); ok {
				return items, nil
			}
		}
	}

	return nil, fmt.Errorf("unsupported response shape")
}

func firstString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := values[key].(string); ok {
			return value
		}
	}

	return ""
}

func firstBool(values map[string]any, keys ...string) bool {
	for _, key := range keys {
		if value, ok := values[key].(bool); ok {
			return value
		}
	}

	return false
}

func firstOptionalBool(values map[string]any, keys ...string) *bool {
	for _, key := range keys {
		value, ok := values[key].(bool)
		if !ok {
			continue
		}

		if key == "disabled" || key == "isDisabled" {
			enabled := !value
			return &enabled
		}

		enabled := value
		return &enabled
	}

	return nil
}
