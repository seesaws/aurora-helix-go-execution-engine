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
	DispatchID          string         `json:"dispatchId"`
	TenantID            string         `json:"tenantId"`
	ExecutionGeneration int64          `json:"executionGeneration"`
	ProcessInstanceID   string         `json:"processInstanceId"`
	NodeInstanceID      string         `json:"nodeInstanceId,omitempty"`
	NodeKey             string         `json:"nodeKey"`
	Status              string         `json:"status"`
	Message             string         `json:"message,omitempty"`
	Output              map[string]any `json:"output,omitempty"`
	CompletedAt         time.Time      `json:"completedAt,omitempty"`
}

type TaskExecutionOutcome struct {
	Status  string         `json:"status"`
	Message string         `json:"message"`
	Output  map[string]any `json:"output,omitempty"`
}

func RunningResult(d DispatchRecord) WorkerResult {
	return WorkerResult{DispatchID: d.DispatchID, TenantID: d.TenantID, ExecutionGeneration: d.ExecutionGeneration, ProcessInstanceID: d.ProcessInstanceID, NodeInstanceID: d.NodeInstanceID, NodeKey: d.NodeKey, Status: ResultStatusRunning}
}
func IsTerminal(status string) bool {
	return status == ResultStatusSucceeded || status == ResultStatusFailed || status == ResultStatusTimeout || status == ResultStatusCancelled
}
