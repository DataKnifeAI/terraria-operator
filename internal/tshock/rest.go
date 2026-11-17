/*
Copyright 2026 DataKnifeAI.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package tshock provides a TShock REST client and GitHub release polling.
package tshock

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client talks to the TShock REST API with token auth.
type Client struct {
	HTTP    *http.Client
	BaseURL string
	Token   string
}

// DefaultHTTP returns an HTTP client with sensible timeouts.
func DefaultHTTP() *http.Client {
	return &http.Client{Timeout: 10 * time.Second}
}

func (c *Client) http() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return DefaultHTTP()
}

// Status is the TShock /status (or /v2/server/status) payload subset.
type Status struct {
	Name            string `json:"name"`
	ServerPassword  bool   `json:"serverpassword"`
	Port            int    `json:"port"`
	PlayerCount     int    `json:"playercount"`
	MaxPlayers      int    `json:"maxplayers"`
	World           string `json:"world"`
	Uptime          string `json:"uptime"`
	ServerSideChars bool   `json:"serversidecharacters"`
}

// TokenCreateResponse is returned by /v2/token/create/{password}.
type TokenCreateResponse struct {
	Status  string `json:"status"`
	Token   string `json:"token"`
	Error   string `json:"error"`
	Message string `json:"response"`
}

// GetStatus probes REST readiness. Tries /v2/server/status then /status.
func (c *Client) GetStatus(ctx context.Context) (*Status, error) {
	var out Status
	err := c.getJSON(ctx, "/v2/server/status", &out)
	if err == nil {
		return &out, nil
	}
	if err2 := c.getJSON(ctx, "/status", &out); err2 == nil {
		return &out, nil
	}
	return nil, err
}

// CreateToken bootstraps a REST token using the RestManagerPassword.
// Endpoint: GET /v2/token/create/{password}?username=...
func (c *Client) CreateToken(ctx context.Context, restPassword, username string) (string, error) {
	if username == "" {
		username = "terraria-operator"
	}
	path := "/v2/token/create/" + url.PathEscape(restPassword) + "?username=" + url.QueryEscape(username)
	var out TokenCreateResponse
	if err := c.getJSONNoToken(ctx, path, &out); err != nil {
		return "", err
	}
	if out.Token == "" {
		msg := out.Error
		if msg == "" {
			msg = out.Message
		}
		if msg == "" {
			msg = out.Status
		}
		return "", fmt.Errorf("tshock token create failed: %s", msg)
	}
	return out.Token, nil
}

// Broadcast sends a server broadcast via REST when available.
func (c *Client) Broadcast(ctx context.Context, message string) error {
	path := "/v2/server/broadcast?msg=" + url.QueryEscape(message)
	return c.getDiscard(ctx, path)
}

// Save triggers a world save.
func (c *Client) Save(ctx context.Context) error {
	return c.getDiscard(ctx, "/v2/server/save")
}

// Off shuts the server down gracefully.
func (c *Client) Off(ctx context.Context) error {
	return c.getDiscard(ctx, "/v2/server/off?confirm=true&nosave=false")
}

func (c *Client) getJSON(ctx context.Context, path string, dest any) error {
	return c.doJSON(ctx, path, true, dest)
}

func (c *Client) getJSONNoToken(ctx context.Context, path string, dest any) error {
	return c.doJSON(ctx, path, false, dest)
}

func (c *Client) doJSON(ctx context.Context, path string, withToken bool, dest any) error {
	body, err := c.do(ctx, path, withToken)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, dest); err != nil {
		return fmt.Errorf("decode tshock %s: %w", path, err)
	}
	return nil
}

func (c *Client) getDiscard(ctx context.Context, path string) error {
	_, err := c.do(ctx, path, true)
	return err
}

func (c *Client) do(ctx context.Context, path string, withToken bool) ([]byte, error) {
	base := strings.TrimRight(c.BaseURL, "/")
	u := base + path
	if withToken && c.Token != "" {
		sep := "?"
		if strings.Contains(path, "?") {
			sep = "&"
		}
		u = base + path + sep + "token=" + url.QueryEscape(c.Token)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "DataKnifeAI/terraria-operator")

	resp, err := c.http().Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("tshock %s: HTTP %d: %s", path, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, nil
}
