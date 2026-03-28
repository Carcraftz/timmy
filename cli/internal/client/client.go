package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

type Server struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	TailscaleIP string    `json:"tailscale_ip"`
	SSHUser     string    `json:"ssh_user"`
	Tags        []string  `json:"tags"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	CreatedBy   string    `json:"created_by"`
	UpdatedBy   string    `json:"updated_by"`
	Source      string    `json:"source,omitempty"`
}

type MeResponse struct {
	LoginName   string `json:"login_name"`
	DisplayName string `json:"display_name"`
	NodeName    string `json:"node_name"`
	Tailnet     string `json:"tailnet"`
}

type CreateServerRequest struct {
	Name        string   `json:"name"`
	TailscaleIP string   `json:"tailscale_ip"`
	SSHUser     string   `json:"ssh_user"`
	Tags        []string `json:"tags"`
}

type UpdateServerRequest struct {
	Name        *string   `json:"name,omitempty"`
	TailscaleIP *string   `json:"tailscale_ip,omitempty"`
	SSHUser     *string   `json:"ssh_user,omitempty"`
	Tags        *[]string `json:"tags,omitempty"`
}

type Service interface {
	Me(ctx context.Context) (MeResponse, error)
	ListServers(ctx context.Context, tags []string) ([]Server, error)
	SearchServers(ctx context.Context, query string, tags []string, limit int) ([]Server, error)
	AddServer(ctx context.Context, request CreateServerRequest) (Server, error)
	UpdateServer(ctx context.Context, id int64, request UpdateServerRequest) (Server, error)
	DeleteServer(ctx context.Context, id int64) error
}

type HTTPClient struct {
	baseURL    *url.URL
	httpClient *http.Client
}

func NewHTTPClient(baseURL string, httpClient *http.Client) (*HTTPClient, error) {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}

	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return nil, fmt.Errorf("parse api url: %w", err)
	}

	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("invalid api url %q", baseURL)
	}

	return &HTTPClient{
		baseURL:    parsed,
		httpClient: httpClient,
	}, nil
}

func (c *HTTPClient) Me(ctx context.Context) (MeResponse, error) {
	var response MeResponse
	if err := c.do(ctx, http.MethodGet, "/me", nil, nil, &response); err != nil {
		return MeResponse{}, err
	}
	return response, nil
}

func (c *HTTPClient) ListServers(ctx context.Context, tags []string) ([]Server, error) {
	query := url.Values{}
	for _, tag := range tags {
		query.Add("tag", tag)
	}

	var response struct {
		Servers []Server `json:"servers"`
	}
	if err := c.do(ctx, http.MethodGet, "/servers", query, nil, &response); err != nil {
		return nil, err
	}
	return response.Servers, nil
}

func (c *HTTPClient) SearchServers(ctx context.Context, query string, tags []string, limit int) ([]Server, error) {
	values := url.Values{}
	values.Set("q", query)
	if limit > 0 {
		values.Set("limit", fmt.Sprintf("%d", limit))
	}
	for _, tag := range tags {
		values.Add("tag", tag)
	}

	var response struct {
		Servers []Server `json:"servers"`
	}
	if err := c.do(ctx, http.MethodGet, "/servers/search", values, nil, &response); err != nil {
		return nil, err
	}
	return response.Servers, nil
}

func (c *HTTPClient) AddServer(ctx context.Context, request CreateServerRequest) (Server, error) {
	var response Server
	if err := c.do(ctx, http.MethodPost, "/servers", nil, request, &response); err != nil {
		return Server{}, err
	}
	return response, nil
}

func (c *HTTPClient) UpdateServer(ctx context.Context, id int64, request UpdateServerRequest) (Server, error) {
	var response Server
	if err := c.do(ctx, http.MethodPatch, fmt.Sprintf("/servers/%d", id), nil, request, &response); err != nil {
		return Server{}, err
	}
	return response, nil
}

func (c *HTTPClient) DeleteServer(ctx context.Context, id int64) error {
	return c.do(ctx, http.MethodDelete, fmt.Sprintf("/servers/%d", id), nil, nil, nil)
}

func (c *HTTPClient) do(ctx context.Context, method, endpoint string, query url.Values, body any, dst any) error {
	requestURL := *c.baseURL
	requestURL.Path = path.Join(c.baseURL.Path, endpoint)
	requestURL.RawQuery = query.Encode()

	var bodyReader *bytes.Reader
	if body == nil {
		bodyReader = bytes.NewReader(nil)
	} else {
		payload, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, requestURL.String(), bodyReader)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, endpoint, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		var apiErr struct {
			Error string `json:"error"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&apiErr); err == nil && apiErr.Error != "" {
			return fmt.Errorf("%s %s: %s", method, endpoint, apiErr.Error)
		}
		return fmt.Errorf("%s %s: unexpected status %s", method, endpoint, resp.Status)
	}

	if dst == nil {
		return nil
	}

	if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
		return fmt.Errorf("decode response body: %w", err)
	}

	return nil
}
