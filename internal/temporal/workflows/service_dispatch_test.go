package workflows

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/aurora/aurora-helix-go-execution-engine/internal/domain"
	"github.com/aurora/aurora-helix-go-execution-engine/internal/executor/demo"
	"github.com/aurora/aurora-helix-go-execution-engine/internal/temporal/activities"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/testsuite"
)

func TestServiceDispatchWorkflow(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	env.RegisterWorkflow(ServiceDispatchWorkflow)
	env.RegisterActivityWithOptions(activities.NewAgentExecutor(demo.NewRegistry()).ExecuteTask, activity.RegisterOptions{Name: activities.ExecuteTaskActivityName})

	payload, err := json.Marshal(domain.TaskRequest{
		TaskType: "demo.echo",
		Input: map[string]any{
			"message": "hello temporal",
		},
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	dispatch := domain.DispatchRecord{
		DispatchID:        "dispatch-1",
		ProcessInstanceID: "proc-1",
		NodeKey:           "serviceNode",
		DispatchStatus:    domain.DispatchStatusPlaceholderDispatched,
		Payload:           payload,
		CreatedAt:         time.Now(),
	}

	env.ExecuteWorkflow(ServiceDispatchWorkflow, dispatch)
	if !env.IsWorkflowCompleted() {
		t.Fatal("workflow did not complete")
	}
	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("workflow returned error: %v", err)
	}
	var result domain.WorkerResult
	if err := env.GetWorkflowResult(&result); err != nil {
		t.Fatalf("read workflow result: %v", err)
	}
	if result.ResultStatus != domain.ResultStatusPlaceholderSuccess {
		t.Fatalf("expected success result, got %s", result.ResultStatus)
	}
	if result.ResultID != "dispatch-1:result" {
		t.Fatalf("unexpected result ID: %s", result.ResultID)
	}
	if result.Output["echoedMessage"] != "hello temporal" {
		t.Fatalf("unexpected output: %+v", result.Output)
	}
}
