package app

import (
	"context"

	"github.com/aurora/aurora-helix-go-execution-engine/internal/domain"
	"github.com/aurora/aurora-helix-go-execution-engine/internal/temporal/queues"
	"github.com/aurora/aurora-helix-go-execution-engine/internal/temporal/workflows"
	"go.temporal.io/sdk/client"
)

type TemporalDispatcher struct {
	client client.Client
}

func NewTemporalDispatcher(client client.Client) *TemporalDispatcher {
	return &TemporalDispatcher{client: client}
}

func (d *TemporalDispatcher) Dispatch(ctx context.Context, dispatch domain.DispatchRecord) (domain.WorkerResult, error) {
	run, err := d.client.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID:        dispatch.DispatchID,
		TaskQueue: queues.ServiceWorkerTaskQueue,
	}, workflows.ServiceDispatchWorkflow, dispatch)
	if err != nil {
		return domain.WorkerResult{}, err
	}
	var result domain.WorkerResult
	if err := run.Get(ctx, &result); err != nil {
		return domain.WorkerResult{}, err
	}
	return result, nil
}

func (d *TemporalDispatcher) GetResult(ctx context.Context, dispatchID string) (domain.WorkerResult, error) {
	run := d.client.GetWorkflow(ctx, dispatchID, "")
	var result domain.WorkerResult
	if err := run.Get(ctx, &result); err != nil {
		return domain.WorkerResult{}, err
	}
	return result, nil
}
