package domain

import (
	"encoding/json"
	"time"
)

const DispatchStatusPlaceholderDispatched = "PLACEHOLDER_DISPATCHED"

type DispatchRecord struct {
	DispatchID        string          `json:"dispatchId"`
	ProcessInstanceID string          `json:"processInstanceId"`
	NodeKey           string          `json:"nodeKey"`
	DispatchStatus    string          `json:"dispatchStatus"`
	Payload           json.RawMessage `json:"payload"`
	CreatedAt         time.Time       `json:"createdAt"`
}

type TaskRequest struct {
	TaskType string            `json:"taskType"`
	Input    map[string]any    `json:"input"`
	Metadata map[string]string `json:"metadata,omitempty"`
}
