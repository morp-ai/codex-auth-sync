package backend

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/morp-ai/codex-auth-sync/internal/model"
)

type fakeWebDAV struct {
	mu      sync.Mutex
	data    []byte
	version int
}

func (server *fakeWebDAV) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	username, password, ok := r.BasicAuth()
	if !ok || username != "user" || password != "password" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if r.Method == "OPTIONS" {
		w.Header().Set("DAV", "1, 2")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	server.mu.Lock()
	defer server.mu.Unlock()
	etag := fmt.Sprintf("\"%x\"", sha256.Sum256(server.data))
	switch r.Method {
	case http.MethodGet:
		if server.data == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("ETag", etag)
		_, _ = w.Write(server.data)
	case http.MethodPut:
		if server.data == nil {
			if r.Header.Get("If-None-Match") != "*" {
				w.WriteHeader(http.StatusPreconditionFailed)
				return
			}
		} else if r.Header.Get("If-Match") != etag {
			w.WriteHeader(http.StatusPreconditionFailed)
			return
		}
		data, _ := io.ReadAll(r.Body)
		server.data = data
		server.version++
		w.Header().Set("ETag", strconv.Quote(strconv.Itoa(server.version)))
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func TestWebDAVPushPullAndHealth(t *testing.T) {
	storage := &fakeWebDAV{}
	server := httptest.NewServer(storage)
	defer server.Close()
	client, err := NewWebDAV(server.URL+"/sync", WebDAVAuth{Username: "user", Password: "password"}, 5*time.Second, "test")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if health, err := client.Health(ctx); err != nil || !health.OK {
		t.Fatalf("unexpected health result: health=%+v err=%v", health, err)
	}
	input := model.Event{
		EventID:    "00112233445566778899aabbccddeeff",
		ClusterID:  "cluster",
		NodeID:     "node",
		Kind:       model.KindSettingsSet,
		EntityID:   "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Ciphertext: "encrypted",
	}
	saved, err := client.Push(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Revision != 1 {
		t.Fatalf("expected revision 1, got %d", saved.Revision)
	}
	response, err := client.Pull(ctx, "cluster", 0, 500, 0)
	if err != nil {
		t.Fatal(err)
	}
	if response.LatestRevision != 1 || len(response.Events) != 1 || response.Events[0].EventID != input.EventID {
		t.Fatalf("unexpected WebDAV events: %+v", response)
	}
}

func TestWebDAVRejectsMissingStrongETag(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"version":1,"next_revision":1,"events":[]}`))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	client, err := NewWebDAV(server.URL, WebDAVAuth{BearerToken: "token"}, 5*time.Second, "test")
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Pull(context.Background(), "cluster", 0, 500, 0)
	if !errors.Is(err, ErrWebDAVConditionalWrites) {
		t.Fatalf("expected strong ETag error, got %v", err)
	}
}

func TestWebDAVConcurrentPushesAreSerializedByETag(t *testing.T) {
	storage := &fakeWebDAV{}
	server := httptest.NewServer(storage)
	defer server.Close()
	newClient := func() (*WebDAVClient, error) {
		client, err := NewWebDAV(server.URL, WebDAVAuth{Username: "user", Password: "password"}, 5*time.Second, "test")
		return client, err
	}
	clients := make([]*WebDAVClient, 2)
	for index := range clients {
		var err error
		clients[index], err = newClient()
		if err != nil {
			t.Fatal(err)
		}
	}
	events := []model.Event{
		{
			EventID:    "00112233445566778899aabbccddeeff",
			ClusterID:  "cluster",
			NodeID:     "node-a",
			Kind:       model.KindSettingsSet,
			EntityID:   "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			Ciphertext: "encrypted-a",
		},
		{
			EventID:    "ffeeddccbbaa99887766554433221100",
			ClusterID:  "cluster",
			NodeID:     "node-b",
			Kind:       model.KindSettingsSet,
			EntityID:   "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			Ciphertext: "encrypted-b",
		},
	}
	var wait sync.WaitGroup
	errCh := make(chan error, len(events))
	for index := range events {
		wait.Add(1)
		go func(client *WebDAVClient, event model.Event) {
			defer wait.Done()
			_, err := client.Push(context.Background(), event)
			errCh <- err
		}(clients[index], events[index])
	}
	wait.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	reader, err := newClient()
	if err != nil {
		t.Fatal(err)
	}
	response, err := reader.Pull(context.Background(), "cluster", 0, 500, 0)
	if err != nil {
		t.Fatal(err)
	}
	if response.LatestRevision != 2 || len(response.Events) != 2 {
		t.Fatalf("expected both concurrent events, got %+v", response)
	}
}
