package app

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/aurora/aurora-helix-go-execution-engine/internal/domain"
	"github.com/aurora/aurora-helix-go-execution-engine/internal/ports"
)

type MemoryDispatcher struct {
	executor ports.TaskExecutor
	now      func() time.Time

	mu      sync.RWMutex
	results map[string]domain.WorkerResult
}

func NewMemoryDispatcher(executor ports.TaskExecutor) *MemoryDispatcher {
	return &MemoryDispatcher{
		executor: executor,
		now:      time.Now,
		results:  make(map[string]domain.WorkerResult),
	}
}

func (d *MemoryDispatcher) Dispatch(ctx context.Context, dispatch domain.DispatchRecord) (domain.DispatchAck, error) {
	d.mu.Lock()
	if _, exists := d.results[dispatch.DispatchID]; exists {
		d.mu.Unlock()
		return domain.DispatchAck{DispatchID: dispatch.DispatchID, Status: domain.DispatchStatusDispatched}, nil
	}
	d.results[dispatch.DispatchID] = domain.WorkerResult{
		DispatchID:        dispatch.DispatchID,
		ProcessInstanceID: dispatch.ProcessInstanceID,
		NodeInstanceID:    dispatch.NodeInstanceID,
		NodeKey:           dispatch.NodeKey,
		Status:            domain.ResultStatusRunning,
	}
	d.mu.Unlock()

	go d.execute(context.WithoutCancel(ctx), dispatch)

	return domain.DispatchAck{DispatchID: dispatch.DispatchID, Status: domain.DispatchStatusDispatched}, nil
}

func (d *MemoryDispatcher) GetResult(_ context.Context, dispatchID string) (domain.WorkerResult, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	result, ok := d.results[dispatchID]
	if !ok {
		return domain.WorkerResult{}, fmt.Errorf("result not found for dispatchId %s", dispatchID)
	}
	return result, nil
}

func (d *MemoryDispatcher) execute(ctx context.Context, dispatch domain.DispatchRecord) {
	payload := dispatch.Payload
	if len(payload) == 0 {
		payload, _ = json.Marshal(domain.TaskRequest{
			TaskType: dispatch.TaskType,
			Input:    dispatch.Input,
			Metadata: dispatch.Metadata,
		})
	}

	var task domain.TaskRequest
	if err := json.Unmarshal(payload, &task); err != nil {
		d.storeFailure(dispatch, "invalid dispatch payload: "+err.Error())
		return
	}
	if task.TaskType == "" {
		task.TaskType = dispatch.TaskType
	}
	if task.Input == nil {
		task.Input = dispatch.Input
	}
	if task.Metadata == nil {
		task.Metadata = dispatch.Metadata
	}

	outcome, err := d.executor.Execute(ctx, task)
	if err != nil {
		d.storeFailure(dispatch, err.Error())
		return
	}
	status := outcome.Status
	if status == "" {
		status = domain.ResultStatusSucceeded
	}
	d.store(domain.WorkerResult{
		DispatchID:        dispatch.DispatchID,
		ProcessInstanceID: dispatch.ProcessInstanceID,
		NodeInstanceID:    dispatch.NodeInstanceID,
		NodeKey:           dispatch.NodeKey,
		Status:            status,
		Message:           outcome.Message,
		Output:            outcome.Output,
		CompletedAt:       d.now(),
	})
}

func (d *MemoryDispatcher) storeFailure(dispatch domain.DispatchRecord, message string) {
	d.store(domain.WorkerResult{
		DispatchID:        dispatch.DispatchID,
		ProcessInstanceID: dispatch.ProcessInstanceID,
		NodeInstanceID:    dispatch.NodeInstanceID,
		NodeKey:           dispatch.NodeKey,
		Status:            domain.ResultStatusFailed,
		Message:           message,
		CompletedAt:       d.now(),
	})
}

func (d *MemoryDispatcher) store(result domain.WorkerResult) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.results[result.DispatchID] = result
}
