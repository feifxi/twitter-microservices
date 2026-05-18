package keycloak

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

var _ Admin = (*Client)(nil)

type kcUpdateUsernameBody struct {
	Username string `json:"username"`
}

type Admin interface {
	UpdateUsername(ctx context.Context, keycloakID, username string) error
}

type Client struct {
	baseURL      string
	realm        string
	clientID     string
	clientSecret string
	http         *http.Client
	log          *slog.Logger

	mu          sync.Mutex
	cachedToken string
	tokenExpiry time.Time
}

func New(baseURL, realm, clientID, clientSecret string, log *slog.Logger) *Client {
	return &Client{
		baseURL:      strings.TrimRight(baseURL, "/"),
		realm:        realm,
		clientID:     clientID,
		clientSecret: clientSecret,
		http:         &http.Client{Timeout: 10 * time.Second},
		log:          log,
	}
}

func (c *Client) UpdateUsername(ctx context.Context, keycloakID, username string) error {
	token, err := c.adminToken(ctx)
	if err != nil {
		return fmt.Errorf("keycloak admin token: %w", err)
	}

	body, err := json.Marshal(kcUpdateUsernameBody{Username: username})
	if err != nil {
		return fmt.Errorf("keycloak: marshal update user payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut,
		c.baseURL+"/admin/realms/"+c.realm+"/users/"+keycloakID, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("keycloak: build update user request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("keycloak update user: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("keycloak update user: status %d: %s", resp.StatusCode, b)
	}
	return nil
}

func (c *Client) adminToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cachedToken != "" && time.Now().Before(c.tokenExpiry) {
		return c.cachedToken, nil
	}

	form := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {c.clientID},
		"client_secret": {c.clientSecret},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/realms/"+c.realm+"/protocol/openid-connect/token",
		strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("keycloak: build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("keycloak token request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("keycloak token: status %d: %s", resp.StatusCode, b)
	}

	var tok struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return "", fmt.Errorf("keycloak token: decode response: %w", err)
	}

	ttl := time.Duration(tok.ExpiresIn)*time.Second - 30*time.Second
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	c.cachedToken = tok.AccessToken
	c.tokenExpiry = time.Now().Add(ttl)
	return tok.AccessToken, nil
}
