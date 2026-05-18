package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/aurora/aurora-helix-go-execution-engine/internal/domain"
	"github.com/aurora/aurora-helix-go-execution-engine/internal/temporal/queues"
	"github.com/aurora/aurora-helix-go-execution-engine/internal/temporal/workflows"
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/sdk/client"
)

type TemporalDispatcher struct {
	client client.Client
}

func NewTemporalDispatcher(client client.Client) *TemporalDispatcher {
	return &TemporalDispatcher{client: client}
}

func (d *TemporalDispatcher) Dispatch(ctx context.Context, dispatch domain.DispatchRecord) (domain.DispatchAck, error) {
	_, err := d.client.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID:        dispatch.DispatchID,
		TaskQueue: queues.ServiceWorkerTaskQueue,
	}, workflows.ServiceDispatchWorkflow, dispatch)
	if err != nil {
		var alreadyStarted *serviceerror.WorkflowExecutionAlreadyStarted
		if errors.As(err, &alreadyStarted) {
			return domain.DispatchAck{DispatchID: dispatch.DispatchID, Status: domain.DispatchStatusDispatched}, nil
		}
		return domain.DispatchAck{}, err
	}
	return domain.DispatchAck{DispatchID: dispatch.DispatchID, Status: domain.DispatchStatusDispatched}, nil
}

func (d *TemporalDispatcher) GetResult(ctx context.Context, dispatchID string) (domain.WorkerResult, error) {
	desc, err := d.client.DescribeWorkflowExecution(ctx, dispatchID, "")
	if err != nil {
		return domain.WorkerResult{}, err
	}

	status := desc.GetWorkflowExecutionInfo().GetStatus()
	if status == enumspb.WORKFLOW_EXECUTION_STATUS_RUNNING {
		return domain.WorkerResult{DispatchID: dispatchID, Status: domain.ResultStatusRunning}, nil
	}

	run := d.client.GetWorkflow(ctx, dispatchID, "")
	var result domain.WorkerResult
	if err := run.Get(ctx, &result); err != nil {
		switch status {
		case enumspb.WORKFLOW_EXECUTION_STATUS_TIMED_OUT:
			return domain.WorkerResult{DispatchID: dispatchID, Status: domain.ResultStatusTimeout, Message: err.Error()}, nil
		case enumspb.WORKFLOW_EXECUTION_STATUS_CANCELED, enumspb.WORKFLOW_EXECUTION_STATUS_TERMINATED:
			return domain.WorkerResult{DispatchID: dispatchID, Status: domain.ResultStatusCancelled, Message: err.Error()}, nil
		default:
			return domain.WorkerResult{DispatchID: dispatchID, Status: domain.ResultStatusFailed, Message: err.Error()}, nil
		}
	}
	if result.Status == "" {
		result.Status = domain.ResultStatusSucceeded
	}
	if result.DispatchID == "" {
		result.DispatchID = dispatchID
	}
	if result.Status != domain.ResultStatusSucceeded && result.Status != domain.ResultStatusFailed && result.Status != domain.ResultStatusTimeout && result.Status != domain.ResultStatusRunning {
		return domain.WorkerResult{}, fmt.Errorf("unsupported workflow result status %q", result.Status)
	}
	return result, nil
}
