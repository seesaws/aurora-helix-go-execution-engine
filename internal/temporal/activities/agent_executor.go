package activities

import (
	"context"
	"encoding/json"

	"github.com/aurora/aurora-helix-go-execution-engine/internal/domain"
	"github.com/aurora/aurora-helix-go-execution-engine/internal/ports"
)

type AgentExecutor struct {
	registry ports.TaskExecutor
}

func NewAgentExecutor(registry ports.TaskExecutor) *AgentExecutor {
	return &AgentExecutor{registry: registry}
}

func (a *AgentExecutor) ExecuteTask(ctx context.Context, dispatch domain.DispatchRecord) (domain.TaskExecutionOutcome, error) {
	task := domain.TaskRequest{
		TaskType: dispatch.TaskType,
		Input:    dispatch.Input,
		Metadata: dispatch.Metadata,
	}
	if len(dispatch.Payload) > 0 {
		if err := json.Unmarshal(dispatch.Payload, &task); err != nil {
			return domain.TaskExecutionOutcome{
				Status:  domain.ResultStatusFailed,
				Message: "invalid dispatch payload: " + err.Error(),
			}, nil
		}
	}
	return a.registry.Execute(ctx, task)
}
