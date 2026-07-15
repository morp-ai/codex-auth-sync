package backend

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/morp-ai/codex-auth-sync/internal/model"
)

const maxResponseBytes = 8 << 20

type Client struct {
	baseURL   *url.URL
	apiToken  string
	http      *http.Client
	userAgent string
}

func New(serverURL, apiToken string, timeout time.Duration, userAgent string) (*Client, error) {
	parsed, err := url.Parse(strings.TrimRight(serverURL, "/"))
	if err != nil {
		return nil, fmt.Errorf("parse server URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, errors.New("server URL must use http or https")
	}
	if parsed.Host == "" {
		return nil, errors.New("server URL must include a host")
	}
	return &Client{
		baseURL:   parsed,
		apiToken:  apiToken,
		http:      &http.Client{Timeout: timeout},
		userAgent: userAgent,
	}, nil
}

func (c *Client) Push(ctx context.Context, event model.Event) (model.Event, error) {
	body, err := json.Marshal(event)
	if err != nil {
		return model.Event{}, err
	}
	request, err := c.request(ctx, http.MethodPost, "/v1/events", bytes.NewReader(body))
	if err != nil {
		return model.Event{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	var saved model.Event
	if err := c.doJSON(request, &saved); err != nil {
		return model.Event{}, err
	}
	return saved, nil
}

func (c *Client) Pull(ctx context.Context, clusterID string, after int64, limit int, wait time.Duration) (model.EventsResponse, error) {
	query := url.Values{}
	query.Set("cluster_id", clusterID)
	query.Set("after", strconv.FormatInt(after, 10))
	query.Set("limit", strconv.Itoa(limit))
	if wait > 0 {
		query.Set("wait", strconv.Itoa(int(wait.Seconds())))
	}
	request, err := c.request(ctx, http.MethodGet, "/v1/events?"+query.Encode(), nil)
	if err != nil {
		return model.EventsResponse{}, err
	}
	var response model.EventsResponse
	if err := c.doJSON(request, &response); err != nil {
		return model.EventsResponse{}, err
	}
	return response, nil
}

func (c *Client) Health(ctx context.Context) (model.HealthResponse, error) {
	request, err := c.request(ctx, http.MethodGet, "/v1/health", nil)
	if err != nil {
		return model.HealthResponse{}, err
	}
	var response model.HealthResponse
	if err := c.doJSON(request, &response); err != nil {
		return model.HealthResponse{}, err
	}
	return response, nil
}

func (c *Client) request(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	endpoint := *c.baseURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + strings.SplitN(path, "?", 2)[0]
	if parts := strings.SplitN(path, "?", 2); len(parts) == 2 {
		endpoint.RawQuery = parts[1]
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint.String(), body)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+c.apiToken)
	if c.userAgent != "" {
		request.Header.Set("User-Agent", c.userAgent)
	}
	return request, nil
}

func (c *Client) doJSON(request *http.Request, target interface{}) error {
	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes))
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode > 299 {
		message := strings.TrimSpace(string(body))
		if message == "" {
			message = http.StatusText(response.StatusCode)
		}
		return fmt.Errorf("sync server returned HTTP %d: %s", response.StatusCode, message)
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("decode sync server response: %w", err)
	}
	return nil
}
