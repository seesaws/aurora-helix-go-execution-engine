package app

import (
	"context"

	"github.com/aurora/aurora-helix-go-execution-engine/internal/domain"
)

type Dispatcher interface {
	Dispatch(ctx context.Context, dispatch domain.DispatchRecord) (domain.WorkerResult, error)
	GetResult(ctx context.Context, dispatchID string) (domain.WorkerResult, error)
}
