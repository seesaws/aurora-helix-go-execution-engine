package ports

import (
	"context"

	"github.com/aurora/aurora-helix-go-execution-engine/internal/domain"
)

type TaskExecutor interface {
	Execute(ctx context.Context, task domain.TaskRequest) (domain.TaskExecutionOutcome, error)
}
