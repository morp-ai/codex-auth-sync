package hub

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/morp-ai/codex-auth-sync/internal/model"
)

const maxRequestBytes = 4 << 20

type Server struct {
	store     *Store
	tokenHash [sha256.Size]byte
}

func NewServer(store *Store, apiToken string) (*Server, error) {
	if len(strings.TrimSpace(apiToken)) < 20 {
		return nil, errors.New("API token must contain at least 20 characters")
	}
	return &Server{store: store, tokenHash: sha256.Sum256([]byte(apiToken))}, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/health", s.handleHealth)
	mux.HandleFunc("/v1/events", s.handleEvents)
	return securityHeaders(mux)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, model.HealthResponse{OK: true, Service: "codex-auth-sync", Protocol: model.ProtocolVersion})
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(r) {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.handleListEvents(w, r)
	case http.MethodPost:
		s.handleAppendEvent(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleListEvents(w http.ResponseWriter, r *http.Request) {
	clusterID := strings.TrimSpace(r.URL.Query().Get("cluster_id"))
	if clusterID == "" {
		writeError(w, http.StatusBadRequest, "cluster_id is required")
		return
	}
	after, err := parseIntQuery(r, "after", 0, 0, 1<<62)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	limit, err := parseIntQueryInt(r, "limit", 500, 1, 500)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	waitSeconds, err := parseIntQuery(r, "wait", 0, 0, 30)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	events, latest, notify := s.store.List(clusterID, after, limit)
	if len(events) == 0 && waitSeconds > 0 {
		select {
		case <-notify:
			events, latest, _ = s.store.List(clusterID, after, limit)
		case <-time.After(time.Duration(waitSeconds) * time.Second):
		case <-r.Context().Done():
			return
		}
	}
	writeJSON(w, http.StatusOK, model.EventsResponse{Events: events, LatestRevision: latest})
}

func (s *Server) handleAppendEvent(w http.ResponseWriter, r *http.Request) {
	body := http.MaxBytesReader(w, r.Body, maxRequestBytes)
	defer body.Close()
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	var event model.Event
	if err := decoder.Decode(&event); err != nil {
		writeError(w, http.StatusBadRequest, "invalid event JSON")
		return
	}
	if err := ensureJSONEOF(decoder); err != nil {
		writeError(w, http.StatusBadRequest, "invalid trailing JSON data")
		return
	}
	event.Revision = 0
	event.CreatedAt = ""
	if err := validateEvent(event); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	saved, err := s.store.Append(event)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to persist event")
		return
	}
	writeJSON(w, http.StatusCreated, saved)
}

func (s *Server) authorized(r *http.Request) bool {
	value := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(value, "Bearer ") {
		return false
	}
	provided := sha256.Sum256([]byte(strings.TrimSpace(strings.TrimPrefix(value, "Bearer "))))
	return subtle.ConstantTimeCompare(provided[:], s.tokenHash[:]) == 1
}

func validateEvent(event model.Event) error {
	if len(event.EventID) != 32 {
		return errors.New("event_id must be a 32-character hexadecimal identifier")
	}
	if _, err := hex.DecodeString(event.EventID); err != nil {
		return errors.New("event_id must be hexadecimal")
	}
	if event.ClusterID == "" || len(event.ClusterID) > 128 {
		return errors.New("cluster_id must contain 1 to 128 characters")
	}
	if event.NodeID == "" || len(event.NodeID) > 128 {
		return errors.New("node_id must contain 1 to 128 characters")
	}
	if !model.ValidKind(event.Kind) {
		return errors.New("unsupported event kind")
	}
	if len(event.EntityID) != 64 {
		return errors.New("entity_id must be a SHA-256 hexadecimal identifier")
	}
	if _, err := hex.DecodeString(event.EntityID); err != nil {
		return errors.New("entity_id must be hexadecimal")
	}
	if event.Ciphertext == "" || len(event.Ciphertext) > maxRequestBytes-1024 {
		return errors.New("ciphertext has an invalid size")
	}
	return nil
}

func parseIntQuery(r *http.Request, name string, fallback, minimum, maximum int64) (int64, error) {
	value := strings.TrimSpace(r.URL.Query().Get(name))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < minimum || parsed > maximum {
		return 0, fmt.Errorf("%s must be between %d and %d", name, minimum, maximum)
	}
	return parsed, nil
}

func parseIntQueryInt(r *http.Request, name string, fallback, minimum, maximum int) (int, error) {
	value := strings.TrimSpace(r.URL.Query().Get(name))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < minimum || parsed > maximum {
		return 0, fmt.Errorf("%s must be between %d and %d", name, minimum, maximum)
	}
	return parsed, nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra interface{}
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	return err
}

func writeJSON(w http.ResponseWriter, status int, value interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(w, r)
	})
}

func RunHTTPServer(ctx context.Context, address, certFile, keyFile string, handler http.Handler) error {
	server := &http.Server{
		Addr:              address,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		if certFile != "" || keyFile != "" {
			errCh <- server.ListenAndServeTLS(certFile, keyFile)
			return
		}
		errCh <- server.ListenAndServe()
	}()
	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	}
}
