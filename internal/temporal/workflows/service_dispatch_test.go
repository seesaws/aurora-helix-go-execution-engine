package workflows

import (
	"context"
	"encoding/json"
	"errors"
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
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
	env.RegisterActivityWithOptions(func(context.Context, domain.WorkerResult) error { return nil }, activity.RegisterOptions{Name: "SOP.DeliverResult"})
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
		DispatchID:          "dispatch-1",
		TenantID:            "tenant",
		ExecutionGeneration: 1,
		ProcessInstanceID:   "proc-1",
		NodeInstanceID:      "node-1",
		NodeKey:             "serviceNode",
		TaskType:            "demo.echo",
		DispatchStatus:      domain.DispatchStatusDispatched,
		Payload:             payload,
		CreatedAt:           time.Now(),
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
	if result.Status != domain.ResultStatusSucceeded {
		t.Fatalf("expected success result, got %s", result.Status)
	}
	if result.DispatchID != "dispatch-1" || result.NodeInstanceID != "node-1" {
		t.Fatalf("unexpected result identity fields: %+v", result)
	}
	if result.Output["echoedMessage"] != "hello temporal" {
		t.Fatalf("unexpected output: %+v", result.Output)
	}
}

func TestTerminalResultQueryableWhileCallbackRetries(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	env.RegisterWorkflow(ServiceDispatchWorkflow)
	env.RegisterActivityWithOptions(func(context.Context, domain.DispatchRecord) (domain.TaskExecutionOutcome, error) {
		return domain.TaskExecutionOutcome{Status: "SUCCEEDED", Output: map[string]any{"echoedMessage": "safe"}}, nil
	}, activity.RegisterOptions{Name: activities.ExecuteTaskActivityName})
	attempts := 0
	env.RegisterActivityWithOptions(func(_ context.Context, result domain.WorkerResult) error {
		attempts++
		value, err := env.QueryWorkflow("dispatch-result")
		if err != nil {
			return err
		}
		var queried domain.WorkerResult
		if err = value.Get(&queried); err != nil {
			return err
		}
		if queried.Status != "SUCCEEDED" || queried.TenantID != "tenant" || queried.ExecutionGeneration != 7 {
			t.Errorf("terminal result not queryable: %+v", queried)
		}
		if attempts == 1 {
			return errors.New("temporary callback outage")
		}
		return nil
	}, activity.RegisterOptions{Name: "SOP.DeliverResult"})
	env.ExecuteWorkflow(ServiceDispatchWorkflow, domain.DispatchRecord{DispatchID: "retry", TenantID: "tenant", ExecutionGeneration: 7, TaskType: "system.echo"})
	if err := env.GetWorkflowError(); err != nil {
		t.Fatal(err)
	}
	if attempts != 2 {
		t.Fatalf("expected durable callback retry, got %d", attempts)
	}
}

func TestCancellationAndTimeoutAreTerminalAndDelivered(t *testing.T) {
	for _, status := range []string{"CANCELLED", "TIMEOUT", "FAILED"} {
		t.Run(status, func(t *testing.T) {
			var suite testsuite.WorkflowTestSuite
			env := suite.NewTestWorkflowEnvironment()
			env.RegisterWorkflow(ServiceDispatchWorkflow)
			calls := 0
			delivered := 0
			env.RegisterActivityWithOptions(func(ctx context.Context, _ domain.DispatchRecord) (domain.TaskExecutionOutcome, error) {
				calls++
				switch status {
				case "CANCELLED":
					return domain.TaskExecutionOutcome{}, temporal.NewCanceledError()
				case "TIMEOUT":
					return domain.TaskExecutionOutcome{}, temporal.NewTimeoutError(enumspb.TIMEOUT_TYPE_START_TO_CLOSE, nil)
				default:
					return domain.TaskExecutionOutcome{}, temporal.NewNonRetryableApplicationError("invalid task", "INVALID_TASK", nil)
				}
			}, activity.RegisterOptions{Name: activities.ExecuteTaskActivityName})
			env.RegisterActivityWithOptions(func(_ context.Context, result domain.WorkerResult) error {
				delivered++
				if result.Status != status || result.TenantID != "tenant" {
					t.Errorf("wrong terminal callback %+v", result)
				}
				return nil
			}, activity.RegisterOptions{Name: activities.DeliverResultActivityName})
			env.ExecuteWorkflow(ServiceDispatchWorkflow, domain.DispatchRecord{DispatchID: "terminal", TenantID: "tenant", ExecutionGeneration: 1, Retry: domain.RetryPolicy{MaxAttempts: 1}})
			if err := env.GetWorkflowError(); err != nil {
				t.Fatal(err)
			}
			if calls != 1 || delivered != 1 {
				t.Fatalf("calls=%d delivered=%d", calls, delivered)
			}
			var result domain.WorkerResult
			if err := env.GetWorkflowResult(&result); err != nil {
				t.Fatal(err)
			}
			if result.Status != status {
				t.Fatalf("wrong result %+v", result)
			}
		})
	}
}
func TestCancellationDuringDeliveryCannotOverwriteTerminalTaskResult(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	env.RegisterWorkflow(ServiceDispatchWorkflow)
	env.RegisterActivityWithOptions(func(context.Context, domain.DispatchRecord) (domain.TaskExecutionOutcome, error) {
		return domain.TaskExecutionOutcome{Status: "SUCCEEDED"}, nil
	}, activity.RegisterOptions{Name: activities.ExecuteTaskActivityName})
	attempts := 0
	env.RegisterActivityWithOptions(func(_ context.Context, result domain.WorkerResult) error {
		attempts++
		if attempts == 1 {
			env.CancelWorkflow()
			return errors.New("outage")
		}
		if result.Status != "SUCCEEDED" {
			t.Errorf("cancel overwrote completed task: %+v", result)
		}
		return nil
	}, activity.RegisterOptions{Name: activities.DeliverResultActivityName})
	env.ExecuteWorkflow(ServiceDispatchWorkflow, domain.DispatchRecord{DispatchID: "done", TenantID: "tenant", ExecutionGeneration: 1})
	if err := env.GetWorkflowError(); err != nil {
		t.Fatal(err)
	}
	if attempts != 2 {
		t.Fatalf("detached delivery stopped: %d", attempts)
	}
}

func TestScaffoldHistoryRemainsUnscopedAndDoesNotCreateMerchantCallback(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	env.RegisterWorkflow(ServiceDispatchWorkflow)
	env.OnGetVersion("merchant-sop-execution-v2", workflow.DefaultVersion, workflow.Version(1)).Return(workflow.DefaultVersion)
	env.RegisterActivityWithOptions(func(context.Context, domain.DispatchRecord) (domain.TaskExecutionOutcome, error) {
		return domain.TaskExecutionOutcome{Status: "SUCCEEDED"}, nil
	}, activity.RegisterOptions{Name: activities.ExecuteTaskActivityName})
	env.ExecuteWorkflow(ServiceDispatchWorkflow, domain.DispatchRecord{DispatchID: "legacy", TaskType: "demo.echo"})
	if err := env.GetWorkflowError(); err != nil {
		t.Fatal(err)
	}
	var result domain.WorkerResult
	if err := env.GetWorkflowResult(&result); err != nil {
		t.Fatal(err)
	}
	if result.TenantID != "" || result.Status != "SUCCEEDED" {
		t.Fatalf("legacy envelope was altered %+v", result)
	}
}
