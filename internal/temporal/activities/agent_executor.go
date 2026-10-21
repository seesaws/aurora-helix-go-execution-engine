package activities

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/aurora/aurora-helix-go-execution-engine/internal/domain"
	"github.com/aurora/aurora-helix-go-execution-engine/internal/ports"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"
)

type AgentExecutor struct {
	registry ports.TaskExecutor
}

func NewAgentExecutor(registry ports.TaskExecutor) *AgentExecutor {
	return &AgentExecutor{registry: registry}
}

func (a *AgentExecutor) ExecuteTask(ctx context.Context, dispatch domain.DispatchRecord) (domain.TaskExecutionOutcome, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	// Heartbeats let Temporal deliver cancellation to cooperative task adapters.
	activity.RecordHeartbeat(ctx)
	go func() {
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				activity.RecordHeartbeat(ctx)
			}
		}
	}()

	task := domain.TaskRequest{
		TaskType: dispatch.TaskType,
		Input:    dispatch.Input,
		Metadata: dispatch.Metadata,
	}
	if len(dispatch.Payload) > 0 {
		if err := json.Unmarshal(dispatch.Payload, &task); err != nil {
			return domain.TaskExecutionOutcome{
				Status:  domain.ResultStatusFailed,
				Message: "invalid dispatch payload: " + err.Error(),
			}, nil
		}
	}
	outcome, err := a.registry.Execute(ctx, task)
	var permanent *domain.ExecutionError
	if errors.As(err, &permanent) {
		return domain.TaskExecutionOutcome{}, temporal.NewNonRetryableApplicationError(permanent.Message, permanent.Kind, nil)
	}
	if errors.Is(err, context.Canceled) {
		return domain.TaskExecutionOutcome{}, temporal.NewCanceledError()
	}
	if err != nil {
		return domain.TaskExecutionOutcome{}, temporal.NewApplicationError("task execution failed", "TASK_TRANSIENT")
	}
	return outcome, nil
}
