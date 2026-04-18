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
	var task domain.TaskRequest
	if err := json.Unmarshal(dispatch.Payload, &task); err != nil {
		return domain.TaskExecutionOutcome{
			ResultStatus:  domain.ResultStatusFailed,
			ResultMessage: "invalid dispatch payload: " + err.Error(),
		}, nil
	}
	return a.registry.Execute(ctx, task)
}
