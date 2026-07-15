package backend

import (
	"context"
	"time"

	"github.com/morp-ai/codex-auth-sync/internal/model"
)

type Remote interface {
	Push(ctx context.Context, event model.Event) (model.Event, error)
	Pull(ctx context.Context, clusterID string, after int64, limit int, wait time.Duration) (model.EventsResponse, error)
	Health(ctx context.Context) (model.HealthResponse, error)
}
