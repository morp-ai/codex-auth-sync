package backend

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/morp-ai/codex-auth-sync/internal/model"
)

const maxWebDAVStateBytes = 32 << 20

var ErrWebDAVConditionalWrites = errors.New("WebDAV service does not provide a strong ETag required for safe conditional writes")

type WebDAVAuth struct {
	Username    string
	Password    string
	BearerToken string
}

type webDAVState struct {
	Version      int           `json:"version"`
	NextRevision int64         `json:"next_revision"`
	Events       []model.Event `json:"events"`
}

type WebDAVClient struct {
	baseURL   *url.URL
	auth      WebDAVAuth
	http      *http.Client
	userAgent string
}

func NewWebDAV(serverURL string, auth WebDAVAuth, timeout time.Duration, userAgent string) (*WebDAVClient, error) {
	parsed, err := url.Parse(strings.TrimRight(serverURL, "/"))
	if err != nil {
		return nil, fmt.Errorf("parse WebDAV URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, errors.New("WebDAV URL must use http or https")
	}
	if parsed.Host == "" {
		return nil, errors.New("WebDAV URL must include a host")
	}
	if auth.BearerToken == "" && (auth.Username == "" || auth.Password == "") {
		return nil, errors.New("WebDAV requires username/password or a bearer token")
	}
	return &WebDAVClient{
		baseURL:   parsed,
		auth:      auth,
		http:      &http.Client{Timeout: timeout},
		userAgent: userAgent,
	}, nil
}

func (client *WebDAVClient) Push(ctx context.Context, event model.Event) (model.Event, error) {
	for attempt := 0; attempt < 12; attempt++ {
		state, etag, exists, err := client.getState(ctx, event.ClusterID)
		if err != nil {
			return model.Event{}, err
		}
		for _, stored := range state.Events {
			if stored.EventID == event.EventID {
				return stored, nil
			}
		}
		event.Revision = state.NextRevision
		event.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		state.NextRevision++
		state.Events = append(state.Events, event)
		data, err := json.MarshalIndent(state, "", "  ")
		if err != nil {
			return model.Event{}, err
		}
		data = append(data, '\n')
		conflict, err := client.putState(ctx, event.ClusterID, data, etag, exists)
		if err != nil {
			return model.Event{}, err
		}
		if conflict {
			if err := waitForRetry(ctx, attempt); err != nil {
				return model.Event{}, err
			}
			continue
		}
		verified, _, _, err := client.getState(ctx, event.ClusterID)
		if err != nil {
			return model.Event{}, err
		}
		for _, stored := range verified.Events {
			if stored.EventID == event.EventID {
				return stored, nil
			}
		}
		if err := waitForRetry(ctx, attempt); err != nil {
			return model.Event{}, err
		}
	}
	return model.Event{}, errors.New("WebDAV state changed repeatedly; retry limit reached")
}

func (client *WebDAVClient) Pull(ctx context.Context, clusterID string, after int64, limit int, _ time.Duration) (model.EventsResponse, error) {
	state, _, _, err := client.getState(ctx, clusterID)
	if err != nil {
		return model.EventsResponse{}, err
	}
	if limit < 1 || limit > 500 {
		limit = 500
	}
	events := make([]model.Event, 0, limit)
	for _, event := range state.Events {
		if event.Revision <= after || event.ClusterID != clusterID {
			continue
		}
		events = append(events, event)
		if len(events) == limit {
			break
		}
	}
	return model.EventsResponse{Events: events, LatestRevision: state.NextRevision - 1}, nil
}

func (client *WebDAVClient) Health(ctx context.Context) (model.HealthResponse, error) {
	request, err := client.request(ctx, "OPTIONS", client.baseURL, nil)
	if err != nil {
		return model.HealthResponse{}, err
	}
	response, err := client.http.Do(request)
	if err != nil {
		return model.HealthResponse{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return model.HealthResponse{}, responseError("WebDAV health check", response)
	}
	return model.HealthResponse{OK: true, Service: "webdav", Protocol: model.ProtocolVersion}, nil
}

func (client *WebDAVClient) getState(ctx context.Context, clusterID string) (webDAVState, string, bool, error) {
	endpoint := client.stateURL(clusterID)
	request, err := client.request(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return webDAVState{}, "", false, err
	}
	request.Header.Set("Cache-Control", "no-cache")
	response, err := client.http.Do(request)
	if err != nil {
		return webDAVState{}, "", false, err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return defaultWebDAVState(), "", false, nil
	}
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return webDAVState{}, "", false, responseError("read WebDAV state", response)
	}
	etag := strings.TrimSpace(response.Header.Get("ETag"))
	if !strongETag(etag) {
		return webDAVState{}, "", false, ErrWebDAVConditionalWrites
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxWebDAVStateBytes+1))
	if err != nil {
		return webDAVState{}, "", false, err
	}
	if len(data) > maxWebDAVStateBytes {
		return webDAVState{}, "", false, errors.New("WebDAV state exceeds the 32 MiB safety limit")
	}
	state := defaultWebDAVState()
	if err := json.Unmarshal(data, &state); err != nil {
		return webDAVState{}, "", false, fmt.Errorf("parse WebDAV sync state: %w", err)
	}
	if state.Version != 1 {
		return webDAVState{}, "", false, fmt.Errorf("unsupported WebDAV state version %d", state.Version)
	}
	if state.NextRevision < 1 {
		state.NextRevision = 1
	}
	for _, event := range state.Events {
		if event.Revision >= state.NextRevision {
			state.NextRevision = event.Revision + 1
		}
	}
	return state, etag, true, nil
}

func (client *WebDAVClient) putState(ctx context.Context, clusterID string, data []byte, etag string, exists bool) (bool, error) {
	endpoint := client.stateURL(clusterID)
	request, err := client.request(ctx, http.MethodPut, endpoint, bytes.NewReader(data))
	if err != nil {
		return false, err
	}
	request.Header.Set("Content-Type", "application/json")
	if exists {
		if !strongETag(etag) {
			return false, ErrWebDAVConditionalWrites
		}
		request.Header.Set("If-Match", etag)
	} else {
		request.Header.Set("If-None-Match", "*")
	}
	response, err := client.http.Do(request)
	if err != nil {
		return false, err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusPreconditionFailed || response.StatusCode == http.StatusConflict {
		return true, nil
	}
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return false, responseError("write WebDAV state", response)
	}
	return false, nil
}

func (client *WebDAVClient) request(ctx context.Context, method string, endpoint *url.URL, body io.Reader) (*http.Request, error) {
	request, err := http.NewRequestWithContext(ctx, method, endpoint.String(), body)
	if err != nil {
		return nil, err
	}
	if client.auth.BearerToken != "" {
		request.Header.Set("Authorization", "Bearer "+client.auth.BearerToken)
	} else {
		request.SetBasicAuth(client.auth.Username, client.auth.Password)
	}
	if client.userAgent != "" {
		request.Header.Set("User-Agent", client.userAgent)
	}
	return request, nil
}

func (client *WebDAVClient) stateURL(clusterID string) *url.URL {
	digest := sha256.Sum256([]byte(clusterID))
	filename := "codex-auth-sync-v1-" + hex.EncodeToString(digest[:]) + ".json"
	endpoint := *client.baseURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/" + filename
	endpoint.RawQuery = ""
	endpoint.Fragment = ""
	return &endpoint
}

func defaultWebDAVState() webDAVState {
	return webDAVState{Version: 1, NextRevision: 1, Events: []model.Event{}}
}

func strongETag(value string) bool {
	return value != "" && value != "*" && !strings.HasPrefix(strings.ToUpper(value), "W/")
}

func waitForRetry(ctx context.Context, attempt int) error {
	delay := time.Duration(20+(attempt*25)) * time.Millisecond
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func responseError(operation string, response *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
	message := strings.TrimSpace(string(body))
	if message == "" {
		message = http.StatusText(response.StatusCode)
	}
	return fmt.Errorf("%s returned HTTP %d: %s", operation, response.StatusCode, message)
}
