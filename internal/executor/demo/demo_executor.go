package demo

import (
	"context"
	"errors"
	"fmt"

	"github.com/aurora/aurora-helix-go-execution-engine/internal/domain"
	"github.com/aurora/aurora-helix-go-execution-engine/internal/ports"
)

type Registry struct {
	executors map[string]ports.TaskExecutor
}

func NewRegistry() *Registry {
	registry := &Registry{executors: map[string]ports.TaskExecutor{}}
	registry.Register("demo.echo", EchoExecutor{})
	registry.Register("supplychain.rfq.create", SupplychainRFQCreateExecutor{})
	return registry
}

func (r *Registry) Register(taskType string, executor ports.TaskExecutor) {
	r.executors[taskType] = executor
}

func (r *Registry) Execute(ctx context.Context, task domain.TaskRequest) (domain.TaskExecutionOutcome, error) {
	executor, ok := r.executors[task.TaskType]
	if !ok {
		return domain.TaskExecutionOutcome{}, fmt.Errorf("unsupported taskType: %s", task.TaskType)
	}
	return executor.Execute(ctx, task)
}

type EchoExecutor struct{}

func (EchoExecutor) Execute(_ context.Context, task domain.TaskRequest) (domain.TaskExecutionOutcome, error) {
	message, ok := task.Input["message"].(string)
	if !ok || message == "" {
		return domain.TaskExecutionOutcome{}, errors.New("demo.echo requires input.message")
	}
	return domain.TaskExecutionOutcome{
		Status:  domain.ResultStatusSucceeded,
		Message: "demo task executed",
		Output: map[string]any{
			"echoedMessage": message,
			"taskType":      task.TaskType,
		},
	}, nil
}

type SupplychainRFQCreateExecutor struct{}

func (SupplychainRFQCreateExecutor) Execute(_ context.Context, task domain.TaskRequest) (domain.TaskExecutionOutcome, error) {
	salesOrderID, ok := task.Input["salesOrderId"].(string)
	if !ok || salesOrderID == "" {
		return domain.TaskExecutionOutcome{}, errors.New("supplychain.rfq.create requires input.salesOrderId")
	}
	return domain.TaskExecutionOutcome{
		Status:  domain.ResultStatusSucceeded,
		Message: "created rfq",
		Output: map[string]any{
			"rfqId":        "RFQ-" + salesOrderID,
			"salesOrderId": salesOrderID,
		},
	}, nil
}
