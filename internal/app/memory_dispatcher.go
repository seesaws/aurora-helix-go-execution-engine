package app

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/aurora/aurora-helix-go-execution-engine/internal/domain"
	"github.com/aurora/aurora-helix-go-execution-engine/internal/ports"
)

// MemoryDispatcher is a local test substitute; runtime entrypoints use Temporal.
type MemoryDispatcher struct {
	executor ports.TaskExecutor
	now      func() time.Time

	mu      sync.RWMutex
	results map[string]domain.WorkerResult
	hashes  map[string]string
	cancels map[string]context.CancelFunc
}

func NewMemoryDispatcher(executor ports.TaskExecutor) *MemoryDispatcher {
	return &MemoryDispatcher{
		executor: executor,
		now:      time.Now,
		results:  make(map[string]domain.WorkerResult),
		hashes:   make(map[string]string),
		cancels:  make(map[string]context.CancelFunc),
	}
}

func (d *MemoryDispatcher) Dispatch(ctx context.Context, dispatch domain.DispatchRecord) (domain.DispatchAck, error) {
	normalized, err := domain.NormalizeDispatch(dispatch)
	if err != nil {
		return domain.DispatchAck{}, err
	}
	dispatch = normalized
	hash, err := domain.SemanticHash(dispatch)
	if err != nil {
		return domain.DispatchAck{}, err
	}
	d.mu.Lock()
	if _, exists := d.results[dispatch.DispatchID]; exists {
		if d.hashes[dispatch.DispatchID] != hash {
			d.mu.Unlock()
			return domain.DispatchAck{}, domain.ErrConflict
		}
		d.mu.Unlock()
		return domain.DispatchAck{DispatchID: dispatch.DispatchID, Status: domain.DispatchStatusDispatched}, nil
	}
	d.hashes[dispatch.DispatchID] = hash
	d.results[dispatch.DispatchID] = domain.WorkerResult{
		DispatchID:          dispatch.DispatchID,
		TenantID:            dispatch.TenantID,
		ExecutionGeneration: dispatch.ExecutionGeneration,
		ProcessInstanceID:   dispatch.ProcessInstanceID,
		NodeInstanceID:      dispatch.NodeInstanceID,
		NodeKey:             dispatch.NodeKey,
		Status:              domain.ResultStatusRunning,
	}
	taskCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	d.cancels[dispatch.DispatchID] = cancel
	d.mu.Unlock()

	go d.execute(taskCtx, dispatch)

	return domain.DispatchAck{DispatchID: dispatch.DispatchID, Status: domain.DispatchStatusDispatched}, nil
}

func (d *MemoryDispatcher) GetResult(_ context.Context, dispatchID string) (domain.WorkerResult, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	result, ok := d.results[dispatchID]
	if !ok {
		return domain.WorkerResult{}, domain.ErrNotFound
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
		DispatchID:          dispatch.DispatchID,
		TenantID:            dispatch.TenantID,
		ExecutionGeneration: dispatch.ExecutionGeneration,
		ProcessInstanceID:   dispatch.ProcessInstanceID,
		NodeInstanceID:      dispatch.NodeInstanceID,
		NodeKey:             dispatch.NodeKey,
		Status:              status,
		Message:             outcome.Message,
		Output:              outcome.Output,
		CompletedAt:         d.now(),
	})
}

func (d *MemoryDispatcher) storeFailure(dispatch domain.DispatchRecord, message string) {
	d.store(domain.WorkerResult{
		DispatchID:          dispatch.DispatchID,
		TenantID:            dispatch.TenantID,
		ExecutionGeneration: dispatch.ExecutionGeneration,
		ProcessInstanceID:   dispatch.ProcessInstanceID,
		NodeInstanceID:      dispatch.NodeInstanceID,
		NodeKey:             dispatch.NodeKey,
		Status:              domain.ResultStatusFailed,
		Message:             message,
		CompletedAt:         d.now(),
	})
}

func (d *MemoryDispatcher) store(result domain.WorkerResult) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if domain.IsTerminal(d.results[result.DispatchID].Status) {
		return
	}
	d.results[result.DispatchID] = result
	if cancel := d.cancels[result.DispatchID]; cancel != nil {
		cancel()
		delete(d.cancels, result.DispatchID)
	}
}

func (d *MemoryDispatcher) Cancel(_ context.Context, id string) (domain.DispatchAck, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	result, ok := d.results[id]
	if !ok {
		return domain.DispatchAck{}, domain.ErrNotFound
	}
	if !domain.IsTerminal(result.Status) {
		result.Status = domain.ResultStatusCancelled
		result.CompletedAt = d.now()
		d.results[id] = result
		if cancel := d.cancels[id]; cancel != nil {
			cancel()
			delete(d.cancels, id)
		}
	}
	return domain.DispatchAck{DispatchID: id, Status: result.Status}, nil
}
