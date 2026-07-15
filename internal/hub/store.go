package hub

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/morp-ai/codex-auth-sync/internal/model"
)

type diskState struct {
	Version      int           `json:"version"`
	NextRevision int64         `json:"next_revision"`
	Events       []model.Event `json:"events"`
}

type Store struct {
	mu       sync.Mutex
	path     string
	state    diskState
	eventIDs map[string]int
	notify   chan struct{}
}

func OpenStore(path string) (*Store, error) {
	store := &Store{
		path: path,
		state: diskState{
			Version:      1,
			NextRevision: 1,
			Events:       []model.Event{},
		},
		eventIDs: make(map[string]int),
		notify:   make(chan struct{}),
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		return store, nil
	}
	if err := json.Unmarshal(data, &store.state); err != nil {
		return nil, fmt.Errorf("parse hub state: %w", err)
	}
	if store.state.Version != 1 {
		return nil, fmt.Errorf("unsupported hub state version %d", store.state.Version)
	}
	if store.state.NextRevision < 1 {
		store.state.NextRevision = 1
	}
	for index, event := range store.state.Events {
		store.eventIDs[event.EventID] = index
		if event.Revision >= store.state.NextRevision {
			store.state.NextRevision = event.Revision + 1
		}
	}
	return store, nil
}

func (s *Store) Append(event model.Event) (model.Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if index, ok := s.eventIDs[event.EventID]; ok {
		return s.state.Events[index], nil
	}
	event.Revision = s.state.NextRevision
	event.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	s.state.NextRevision++
	s.state.Events = append(s.state.Events, event)
	s.eventIDs[event.EventID] = len(s.state.Events) - 1
	if err := s.saveLocked(); err != nil {
		delete(s.eventIDs, event.EventID)
		s.state.Events = s.state.Events[:len(s.state.Events)-1]
		s.state.NextRevision--
		return model.Event{}, err
	}
	close(s.notify)
	s.notify = make(chan struct{})
	return event, nil
}

func (s *Store) List(clusterID string, after int64, limit int) ([]model.Event, int64, <-chan struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit < 1 || limit > 500 {
		limit = 500
	}
	items := make([]model.Event, 0, limit)
	for _, event := range s.state.Events {
		if event.Revision <= after || event.ClusterID != clusterID {
			continue
		}
		items = append(items, event)
		if len(items) == limit {
			break
		}
	}
	return items, s.state.NextRevision - 1, s.notify
}

func (s *Store) EventCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.state.Events)
}

func (s *Store) saveLocked() error {
	data, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(s.path), ".hub-*.tmp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0600); err != nil && runtime.GOOS != "windows" {
		temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		_ = os.Remove(s.path)
	}
	if err := os.Rename(tempPath, s.path); err != nil {
		return err
	}
	if runtime.GOOS != "windows" {
		return os.Chmod(s.path, 0600)
	}
	return nil
}
