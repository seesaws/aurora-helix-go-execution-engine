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
	"go.temporal.io/sdk/converter"
)

type TemporalDispatcher struct{ client client.Client }

func NewTemporalDispatcher(client client.Client) *TemporalDispatcher {
	return &TemporalDispatcher{client: client}
}

func (d *TemporalDispatcher) Dispatch(ctx context.Context, dispatch domain.DispatchRecord) (domain.DispatchAck, error) {
	normalized, err := domain.NormalizeDispatch(dispatch)
	if err != nil {
		return domain.DispatchAck{}, err
	}
	hash, err := domain.SemanticHash(normalized)
	if err != nil {
		return domain.DispatchAck{}, err
	}
	_, err = d.client.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID: dispatch.DispatchID, TaskQueue: queues.ServiceWorkerTaskQueue,
		// The workflow ID is the durable dispatch identity, even after completion.
		WorkflowIDReusePolicy:                    enumspb.WORKFLOW_ID_REUSE_POLICY_REJECT_DUPLICATE,
		WorkflowExecutionErrorWhenAlreadyStarted: true,
		Memo:                                     map[string]interface{}{"semanticHash": hash, "identity": domain.RunningResult(normalized)},
	}, workflows.ServiceDispatchWorkflow, normalized)
	if err != nil {
		var duplicate *serviceerror.WorkflowExecutionAlreadyStarted
		if !errors.As(err, &duplicate) {
			return domain.DispatchAck{}, err
		}
		description, err := d.client.DescribeWorkflowExecution(ctx, dispatch.DispatchID, "")
		if err != nil {
			return domain.DispatchAck{}, err
		}
		var original string
		payload := description.GetWorkflowExecutionInfo().GetMemo().GetFields()["semanticHash"]
		if payload == nil || converter.GetDefaultDataConverter().FromPayload(payload, &original) != nil || original != hash {
			return domain.DispatchAck{}, domain.ErrConflict
		}
	}
	return domain.DispatchAck{DispatchID: dispatch.DispatchID, Status: domain.DispatchStatusDispatched}, nil
}

func (d *TemporalDispatcher) GetResult(ctx context.Context, id string) (domain.WorkerResult, error) {
	description, err := d.client.DescribeWorkflowExecution(ctx, id, "")
	if err != nil {
		return domain.WorkerResult{}, lookupError(err)
	}
	info := description.GetWorkflowExecutionInfo()
	var identity domain.WorkerResult
	if payload := info.GetMemo().GetFields()["identity"]; payload != nil {
		_ = converter.GetDefaultDataConverter().FromPayload(payload, &identity)
	}
	if info.GetStatus() == enumspb.WORKFLOW_EXECUTION_STATUS_RUNNING {
		value, err := d.client.QueryWorkflow(ctx, id, info.GetExecution().GetRunId(), workflows.ResultQueryName)
		if err == nil {
			var result domain.WorkerResult
			if err = value.Get(&result); err != nil {
				return identity, err
			}
			return result, nil
		}
		// A queued workflow may not have installed its query handler yet. Its
		// immutable memo still supplies the full envelope; other query errors fail.
		var queryFailed *serviceerror.QueryFailed
		if errors.As(err, &queryFailed) && identity.DispatchID != "" {
			return identity, nil
		}
		return identity, err
	}
	run := d.client.GetWorkflow(ctx, id, info.GetExecution().GetRunId())
	var result domain.WorkerResult
	if err = run.Get(ctx, &result); err != nil {
		// Unexpected workflow termination remains visible with its trusted identity.
		switch info.GetStatus() {
		case enumspb.WORKFLOW_EXECUTION_STATUS_TIMED_OUT:
			identity.Status = domain.ResultStatusTimeout
		case enumspb.WORKFLOW_EXECUTION_STATUS_CANCELED, enumspb.WORKFLOW_EXECUTION_STATUS_TERMINATED:
			identity.Status = domain.ResultStatusCancelled
		default:
			identity.Status = domain.ResultStatusFailed
		}
		identity.Message = "workflow ended without a task result"
		return identity, nil
	}
	if !domain.IsTerminal(result.Status) {
		return result, fmt.Errorf("invalid workflow result")
	}
	return result, nil
}
func (d *TemporalDispatcher) Cancel(ctx context.Context, id string) (domain.DispatchAck, error) {
	result, err := d.GetResult(ctx, id)
	if err != nil {
		return domain.DispatchAck{}, err
	}
	if domain.IsTerminal(result.Status) {
		return domain.DispatchAck{DispatchID: id, Status: result.Status}, nil
	}
	if err = d.client.CancelWorkflow(ctx, id, ""); err != nil {
		return domain.DispatchAck{}, lookupError(err)
	}
	return domain.DispatchAck{DispatchID: id, Status: "CANCEL_REQUESTED"}, nil
}
func lookupError(err error) error {
	var missing *serviceerror.NotFound
	if errors.As(err, &missing) {
		return domain.ErrNotFound
	}
	return err
}
