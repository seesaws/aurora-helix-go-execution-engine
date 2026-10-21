package workflows

import (
	"time"

	"github.com/aurora/aurora-helix-go-execution-engine/internal/domain"
	"github.com/aurora/aurora-helix-go-execution-engine/internal/temporal/activities"
	"github.com/aurora/aurora-helix-go-execution-engine/internal/temporal/queues"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

const ResultQueryName = "dispatch-result"

// ServiceDispatchWorkflow executes exactly one service task. Java owns the SOP
// graph and business state. Terminal results are readable before callback delivery.
func ServiceDispatchWorkflow(ctx workflow.Context, dispatch domain.DispatchRecord) (domain.WorkerResult, error) {
	// Histories from the scaffold retain their original activity/terminal path.
	// They have no trusted tenant envelope and must never gain an M0 callback.
	if workflow.GetVersion(ctx, "merchant-sop-execution-v2", workflow.DefaultVersion, 1) == workflow.DefaultVersion || dispatch.TenantID == "" {
		return legacyServiceDispatchWorkflow(ctx, dispatch)
	}
	result := domain.RunningResult(dispatch)
	if err := workflow.SetQueryHandler(ctx, ResultQueryName, func() (domain.WorkerResult, error) { return result, nil }); err != nil {
		return result, err
	}
	timeout := 30 * time.Second
	if dispatch.TimeoutSeconds > 0 {
		timeout = time.Duration(dispatch.TimeoutSeconds) * time.Second
	}
	attempts := int32(3)
	if dispatch.Retry.MaxAttempts > 0 {
		attempts = int32(dispatch.Retry.MaxAttempts)
	}
	backoff, err := domain.BackoffDuration(dispatch.Retry.Backoff)
	if err != nil {
		return result, err
	}
	taskCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		TaskQueue:              queues.AgentExecutorTaskQueue,
		StartToCloseTimeout:    timeout,
		ScheduleToCloseTimeout: timeout,
		WaitForCancellation:    true,
		HeartbeatTimeout:       time.Second,
		RetryPolicy:            &temporal.RetryPolicy{InitialInterval: backoff, BackoffCoefficient: 2, MaximumInterval: time.Hour, MaximumAttempts: attempts, NonRetryableErrorTypes: []string{"INVALID_TASK", "UNSUPPORTED_TASK"}},
	})
	var outcome domain.TaskExecutionOutcome
	err = workflow.ExecuteActivity(taskCtx, activities.ExecuteTaskActivityName, dispatch).Get(taskCtx, &outcome)
	result.Status = outcome.Status
	result.Output = outcome.Output
	result.Message = outcome.Message
	if result.Status == "" {
		result.Status = domain.ResultStatusSucceeded
	}
	// Cancellation has priority until the task's terminal result is recorded.
	// Later cancellation cannot overwrite this terminal result during delivery.
	switch {
	case ctx.Err() != nil || temporal.IsCanceledError(err):
		result.Status = domain.ResultStatusCancelled
		result.Message = "task cancelled"
		result.Output = nil
	case temporal.IsTimeoutError(err):
		result.Status = domain.ResultStatusTimeout
		result.Message = "task timed out"
		result.Output = nil
	case err != nil:
		result.Status = domain.ResultStatusFailed
		result.Message = "task execution failed"
		result.Output = nil
	case !domain.IsTerminal(result.Status):
		result.Status = domain.ResultStatusFailed
		result.Message = "invalid task outcome"
		result.Output = nil
	}
	result.CompletedAt = workflow.Now(ctx)
	// Detached delivery is durable even when cancellation stopped the task.
	deliveryCtx, _ := workflow.NewDisconnectedContext(ctx)
	deliveryCtx = workflow.WithActivityOptions(deliveryCtx, workflow.ActivityOptions{
		TaskQueue: queues.AgentExecutorTaskQueue, StartToCloseTimeout: 15 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{InitialInterval: time.Second, BackoffCoefficient: 2, MaximumInterval: 5 * time.Minute},
	})
	if err = workflow.ExecuteActivity(deliveryCtx, activities.DeliverResultActivityName, result).Get(deliveryCtx, nil); err != nil {
		return result, err
	}
	return result, nil
}
