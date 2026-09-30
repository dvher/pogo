// Package syncclient talks to a Pogo Pad sync server (see server/API.md).
package syncclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var (
	ErrUnauthorized = errors.New("the server rejected the API token")
	ErrNotFound     = errors.New("not found")
	ErrConflict     = errors.New("conflict")
)

// Note mirrors the server's wire type.
type Note struct {
	ID        string `json:"id"`
	Content   string `json:"content"`
	Color     string `json:"color"`
	Deleted   bool   `json:"deleted"`
	UpdatedAt int64  `json:"updated_at"`
	DeviceID  string `json:"device_id"`
	Rev       int64  `json:"rev,omitempty"`
}

type SyncRequest struct {
	Cursor  int64  `json:"cursor"`
	Changes []Note `json:"changes"`
}

type SyncResponse struct {
	Cursor  int64  `json:"cursor"`
	Changes []Note `json:"changes"`
	More    bool   `json:"more"`
}

type E2EParams struct {
	KDF   string `json:"kdf"`
	Salt  string `json:"salt"`
	Check string `json:"check"`
}

type Client struct {
	base  string
	token string
	http  *http.Client
}

// BaseURL builds a server URL from the settings fields. host may be an IP
// or hostname; port may be empty.
func BaseURL(scheme, host, port string) (string, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return "", errors.New("server host is empty")
	}
	if scheme != "http" && scheme != "https" {
		scheme = "http"
	}
	if strings.Contains(host, "://") {
		u, err := url.Parse(host)
		if err != nil {
			return "", err
		}
		scheme, host = u.Scheme, u.Host
	}
	if port = strings.TrimSpace(port); port != "" {
		host = host + ":" + port
	}
	u := url.URL{Scheme: scheme, Host: host}
	if _, err := url.Parse(u.String()); err != nil {
		return "", fmt.Errorf("invalid server address: %w", err)
	}
	return u.String(), nil
}

func New(baseURL, token string) *Client {
	return &Client{
		base:  strings.TrimRight(baseURL, "/"),
		token: token,
		http:  &http.Client{Timeout: 20 * time.Second},
	}
}

func (c *Client) do(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("cannot reach server: %w", err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated, http.StatusNoContent:
	case http.StatusUnauthorized:
		return ErrUnauthorized
	case http.StatusNotFound:
		return ErrNotFound
	case http.StatusConflict:
		return ErrConflict
	default:
		var e struct {
			Error string `json:"error"`
		}
		json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&e)
		if e.Error == "" {
			e.Error = resp.Status
		}
		return fmt.Errorf("server error: %s", e.Error)
	}
	if out != nil && resp.StatusCode != http.StatusNoContent {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

// Health checks that a Pogo Pad server is listening at the base URL.
func (c *Client) Health(ctx context.Context) (version string, err error) {
	var out struct {
		OK      bool   `json:"ok"`
		Version string `json:"version"`
	}
	if err := c.do(ctx, "GET", "/api/v1/health", nil, &out); err != nil {
		if errors.Is(err, ErrNotFound) {
			return "", errors.New("the address responded but is not a Pogo Pad server")
		}
		return "", err
	}
	if !out.OK {
		return "", errors.New("server reported unhealthy")
	}
	return out.Version, nil
}

func (c *Client) Sync(ctx context.Context, req SyncRequest) (SyncResponse, error) {
	var out SyncResponse
	if req.Changes == nil {
		req.Changes = []Note{}
	}
	err := c.do(ctx, "POST", "/api/v1/sync", req, &out)
	return out, err
}

// GetE2E returns the server's end-to-end setup, or ErrNotFound if unset.
func (c *Client) GetE2E(ctx context.Context) (E2EParams, error) {
	var out E2EParams
	err := c.do(ctx, "GET", "/api/v1/e2e", nil, &out)
	return out, err
}

// PutE2E stores the setup; with force it replaces an existing one.
func (c *Client) PutE2E(ctx context.Context, p E2EParams, force bool) error {
	path := "/api/v1/e2e"
	if force {
		path += "?force=1"
	}
	return c.do(ctx, "PUT", path, p, nil)
}

func (c *Client) DeleteE2E(ctx context.Context) error {
	return c.do(ctx, "DELETE", "/api/v1/e2e", nil, nil)
}
