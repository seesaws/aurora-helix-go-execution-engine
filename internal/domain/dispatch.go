package domain

import (
	"encoding/json"
	"time"
)

const DispatchStatusDispatched = "DISPATCHED"

type RetryPolicy struct {
	MaxAttempts int    `json:"maxAttempts,omitempty"`
	Backoff     string `json:"backoff,omitempty"`
}

type DispatchRecord struct {
	DispatchID          string          `json:"dispatchId"`
	TenantID            string          `json:"tenantId"`
	ExecutionGeneration int64           `json:"executionGeneration"`
	ProcessInstanceID   string          `json:"processInstanceId"`
	NodeInstanceID      string          `json:"nodeInstanceId,omitempty"`
	NodeKey             string          `json:"nodeKey"`
	TaskType            string          `json:"taskType"`
	IdempotencyKey      string          `json:"idempotencyKey,omitempty"`
	Input               map[string]any  `json:"input,omitempty"`
	Metadata            map[string]any  `json:"metadata,omitempty"`
	Retry               RetryPolicy     `json:"retry,omitempty"`
	TimeoutSeconds      int             `json:"timeoutSeconds,omitempty"`
	DispatchStatus      string          `json:"dispatchStatus,omitempty"`
	Payload             json.RawMessage `json:"payload,omitempty"`
	CreatedAt           time.Time       `json:"createdAt,omitempty"`
}

type TaskRequest struct {
	TaskType string         `json:"taskType"`
	Input    map[string]any `json:"input"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type DispatchAck struct {
	DispatchID string `json:"dispatchId"`
	Status     string `json:"status"`
}
