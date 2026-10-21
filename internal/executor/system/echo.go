package system

import (
	"context"
	"github.com/aurora/aurora-helix-go-execution-engine/internal/domain"
)

// Executor is the production capability allowlist. It has no business writes.
type Executor struct{}

func (Executor) Execute(ctx context.Context, task domain.TaskRequest) (domain.TaskExecutionOutcome, error) {
	if err := ctx.Err(); err != nil {
		return domain.TaskExecutionOutcome{}, err
	}
	if task.TaskType != "system.echo" {
		return domain.TaskExecutionOutcome{}, &domain.ExecutionError{Kind: "UNSUPPORTED_TASK", Message: "unsupported taskType"}
	}
	return domain.TaskExecutionOutcome{Status: domain.ResultStatusSucceeded, Message: "echo completed", Output: map[string]any{"echoedMessage": task.Input["message"], "taskType": task.TaskType}}, nil
}
