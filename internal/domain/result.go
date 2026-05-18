package domain

import "time"

const (
	ResultStatusRunning   = "RUNNING"
	ResultStatusSucceeded = "SUCCEEDED"
	ResultStatusFailed    = "FAILED"
	ResultStatusTimeout   = "TIMEOUT"
	ResultStatusCancelled = "CANCELLED"
)

type WorkerResult struct {
	DispatchID        string         `json:"dispatchId"`
	ProcessInstanceID string         `json:"processInstanceId"`
	NodeInstanceID    string         `json:"nodeInstanceId,omitempty"`
	NodeKey           string         `json:"nodeKey"`
	Status            string         `json:"status"`
	Message           string         `json:"message,omitempty"`
	Output            map[string]any `json:"output,omitempty"`
	CompletedAt       time.Time      `json:"completedAt,omitempty"`
}

type TaskExecutionOutcome struct {
	Status  string         `json:"status"`
	Message string         `json:"message"`
	Output  map[string]any `json:"output,omitempty"`
}
