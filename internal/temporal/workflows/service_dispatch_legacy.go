package workflows

import (
	"github.com/aurora/aurora-helix-go-execution-engine/internal/domain"
	"github.com/aurora/aurora-helix-go-execution-engine/internal/temporal/activities"
	"github.com/aurora/aurora-helix-go-execution-engine/internal/temporal/queues"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
	"time"
)

// Preserve the scaffold command sequence for replay only. New HTTP dispatches
// cannot enter this unscoped path. Legacy runs never receive an invented tenant
// or a callback into the merchant SOP protocol.
func legacyServiceDispatchWorkflow(ctx workflow.Context, dispatch domain.DispatchRecord) (domain.WorkerResult, error) {
	timeout := 30 * time.Second
	if dispatch.TimeoutSeconds > 0 {
		timeout = time.Duration(dispatch.TimeoutSeconds) * time.Second
	}
	attempts := int32(3)
	if dispatch.Retry.MaxAttempts > 0 {
		attempts = int32(dispatch.Retry.MaxAttempts)
	}
	ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{TaskQueue: queues.AgentExecutorTaskQueue, StartToCloseTimeout: timeout, RetryPolicy: &temporal.RetryPolicy{InitialInterval: time.Second, BackoffCoefficient: 2, MaximumInterval: 10 * time.Second, MaximumAttempts: attempts}})
	var outcome domain.TaskExecutionOutcome
	err := workflow.ExecuteActivity(ctx, activities.ExecuteTaskActivityName, dispatch).Get(ctx, &outcome)
	result := domain.WorkerResult{DispatchID: dispatch.DispatchID, ProcessInstanceID: dispatch.ProcessInstanceID, NodeInstanceID: dispatch.NodeInstanceID, NodeKey: dispatch.NodeKey, Status: outcome.Status, Message: outcome.Message, Output: outcome.Output, CompletedAt: workflow.Now(ctx)}
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
