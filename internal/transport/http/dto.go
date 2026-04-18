package http

import (
	"github.com/aurora/aurora-helix-go-execution-engine/internal/domain"
)

type CreateDispatchRequest struct {
	ProcessInstanceID string             `json:"processInstanceId"`
	NodeKey           string             `json:"nodeKey"`
	Payload           domain.TaskRequest `json:"payload"`
}

type CreateDispatchResponse struct {
	Dispatch domain.DispatchRecord `json:"dispatch"`
	Result   domain.WorkerResult   `json:"result"`
}

type HealthResponse struct {
	Status string `json:"status"`
}
