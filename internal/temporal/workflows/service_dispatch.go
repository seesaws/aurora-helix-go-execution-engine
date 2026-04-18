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
	options := workflow.ActivityOptions{
		TaskQueue:           queues.AgentExecutorTaskQueue,
		StartToCloseTimeout: 30 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2,
			MaximumInterval:    10 * time.Second,
			MaximumAttempts:    3,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, options)

	var outcome domain.TaskExecutionOutcome
	err := workflow.ExecuteActivity(ctx, activities.ExecuteTaskActivityName, dispatch).Get(ctx, &outcome)
	result := domain.WorkerResult{
		ResultID:          dispatch.DispatchID + ":result",
		DispatchID:        dispatch.DispatchID,
		ProcessInstanceID: dispatch.ProcessInstanceID,
		NodeKey:           dispatch.NodeKey,
		ResultStatus:      outcome.ResultStatus,
		ResultMessage:     outcome.ResultMessage,
		Output:            outcome.Output,
		CreatedAt:         workflow.Now(ctx),
	}
	if err != nil {
		result.ResultStatus = domain.ResultStatusFailed
		result.ResultMessage = err.Error()
		result.Output = nil
	}
	return result, nil
}
