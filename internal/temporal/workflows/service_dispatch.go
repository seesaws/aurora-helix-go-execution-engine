package workflows

import (
	"time"

	"github.com/aurora/aurora-helix-go-execution-engine/internal/domain"
	"github.com/aurora/aurora-helix-go-execution-engine/internal/temporal/activities"
	"github.com/aurora/aurora-helix-go-execution-engine/internal/temporal/queues"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

func ServiceDispatchWorkflow(ctx workflow.Context, dispatch domain.DispatchRecord) (domain.WorkerResult, error) {
	timeout := 30 * time.Second
	if dispatch.TimeoutSeconds > 0 {
		timeout = time.Duration(dispatch.TimeoutSeconds) * time.Second
	}
	maxAttempts := int32(3)
	if dispatch.Retry.MaxAttempts > 0 {
		maxAttempts = int32(dispatch.Retry.MaxAttempts)
	}

	options := workflow.ActivityOptions{
		TaskQueue:           queues.AgentExecutorTaskQueue,
		StartToCloseTimeout: timeout,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2,
			MaximumInterval:    10 * time.Second,
			MaximumAttempts:    maxAttempts,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, options)

	var outcome domain.TaskExecutionOutcome
	err := workflow.ExecuteActivity(ctx, activities.ExecuteTaskActivityName, dispatch).Get(ctx, &outcome)
	result := domain.WorkerResult{
		DispatchID:        dispatch.DispatchID,
		ProcessInstanceID: dispatch.ProcessInstanceID,
		NodeInstanceID:    dispatch.NodeInstanceID,
		NodeKey:           dispatch.NodeKey,
		Status:            outcome.Status,
		Message:           outcome.Message,
		Output:            outcome.Output,
		CompletedAt:       workflow.Now(ctx),
	}
	if result.Status == "" {
		result.Status = domain.ResultStatusSucceeded
	}
	if err != nil {
		result.Status = domain.ResultStatusFailed
		result.Message = err.Error()
		result.Output = nil
	}
	return result, nil
}
