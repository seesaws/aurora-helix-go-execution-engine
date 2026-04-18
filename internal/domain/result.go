package domain

import "time"

const (
	ResultStatusPlaceholderSuccess = "PLACEHOLDER_SUCCESS"
	ResultStatusFailed             = "FAILED"
)

type WorkerResult struct {
	ResultID          string         `json:"resultId"`
	DispatchID        string         `json:"dispatchId"`
	ProcessInstanceID string         `json:"processInstanceId"`
	NodeKey           string         `json:"nodeKey"`
	ResultStatus      string         `json:"resultStatus"`
	ResultMessage     string         `json:"resultMessage"`
	Output            map[string]any `json:"output,omitempty"`
	CreatedAt         time.Time      `json:"createdAt"`
}

type TaskExecutionOutcome struct {
	ResultStatus  string         `json:"resultStatus"`
	ResultMessage string         `json:"resultMessage"`
	Output        map[string]any `json:"output,omitempty"`
}
