package http

type CreateDispatchRequest struct {
	DispatchID          string         `json:"dispatchId"`
	TenantID            string         `json:"tenantId"`
	ExecutionGeneration int64          `json:"executionGeneration"`
	ProcessInstanceID   string         `json:"processInstanceId"`
	NodeInstanceID      string         `json:"nodeInstanceId"`
	NodeKey             string         `json:"nodeKey"`
	TaskType            string         `json:"taskType"`
	IdempotencyKey      string         `json:"idempotencyKey"`
	Input               map[string]any `json:"input"`
	Metadata            map[string]any `json:"metadata"`
	Retry               RetryRequest   `json:"retry"`
	TimeoutSeconds      int            `json:"timeoutSeconds"`
}

type RetryRequest struct {
	MaxAttempts int    `json:"maxAttempts"`
	Backoff     string `json:"backoff"`
}

type CreateDispatchResponse struct {
	DispatchID string `json:"dispatchId"`
	Status     string `json:"status"`
}

type HealthResponse struct {
	Status string `json:"status"`
}
