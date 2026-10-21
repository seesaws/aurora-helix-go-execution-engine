package domain

// ExecutionError marks a permanent task rejection. Unclassified executor errors
// are transient and retry according to dispatch policy.
type ExecutionError struct {
	Kind    string
	Message string
}

func (e *ExecutionError) Error() string { return e.Message }
