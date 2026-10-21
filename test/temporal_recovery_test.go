package test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	commonpb "go.temporal.io/api/common/v1"
	enumspb "go.temporal.io/api/enums/v1"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aurora/aurora-helix-go-execution-engine/internal/app"
	"github.com/aurora/aurora-helix-go-execution-engine/internal/domain"
	"github.com/aurora/aurora-helix-go-execution-engine/internal/executor/system"
	"github.com/aurora/aurora-helix-go-execution-engine/internal/temporal/activities"
	"github.com/aurora/aurora-helix-go-execution-engine/internal/temporal/queues"
	"github.com/aurora/aurora-helix-go-execution-engine/internal/temporal/workflows"
	transporthttp "github.com/aurora/aurora-helix-go-execution-engine/internal/transport/http"
	historypb "go.temporal.io/api/history/v1"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
)

// This test talks to an actual Temporal server. It is deliberately separate
// from testsuite coverage. Set TEMPORAL_INTEGRATION=1 with an isolated server.
func TestRealTemporalCallbackRecoveryAndCompletedReplay(t *testing.T) {
	if os.Getenv("TEMPORAL_INTEGRATION") != "1" {
		t.Skip("requires explicit isolated real Temporal server")
	}
	host := os.Getenv("TEMPORAL_HOST_PORT")
	if host == "" {
		host = "127.0.0.1:7233"
	}
	temporalClient, err := client.Dial(client.Options{HostPort: host})
	if err != nil {
		t.Fatal(err)
	}
	defer temporalClient.Close()
	var accepted atomic.Bool
	var executed atomic.Int32
	var callbackAttempts atomic.Int32
	var deliveries atomic.Int32
	callback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callbackAttempts.Add(1)
		if r.Header.Get("Authorization") != "Bearer callback-test" {
			t.Error("wrong callback machine identity")
			w.WriteHeader(401)
			return
		}
		if !accepted.Load() {
			w.WriteHeader(503)
			return
		}
		deliveries.Add(1)
	}))
	defer callback.Close()
	reporter, err := activities.NewCompletionReporter(callback.URL, func(context.Context) (string, error) { return "callback-test", nil }, callback.Client())
	if err != nil {
		t.Fatal(err)
	}
	startWorkers := func() (worker.Worker, worker.Worker) {
		service := worker.New(temporalClient, queues.ServiceWorkerTaskQueue, worker.Options{WorkerStopTimeout: time.Second})
		service.RegisterWorkflow(workflows.ServiceDispatchWorkflow)
		agent := worker.New(temporalClient, queues.AgentExecutorTaskQueue, worker.Options{WorkerStopTimeout: time.Second})
		agent.RegisterActivityWithOptions(func(ctx context.Context, d domain.DispatchRecord) (domain.TaskExecutionOutcome, error) {
			executed.Add(1)
			return system.Executor{}.Execute(ctx, domain.TaskRequest{TaskType: d.TaskType, Input: d.Input})
		}, activity.RegisterOptions{Name: activities.ExecuteTaskActivityName})
		agent.RegisterActivityWithOptions(reporter.DeliverResult, activity.RegisterOptions{Name: activities.DeliverResultActivityName})
		if err := service.Start(); err != nil {
			t.Fatal(err)
		}
		if err := agent.Start(); err != nil {
			service.Stop()
			t.Fatal(err)
		}
		return service, agent
	}
	service, agent := startWorkers()
	dispatcher := app.NewTemporalDispatcher(temporalClient)
	server := httptest.NewServer(transporthttp.NewServer(slog.New(slog.NewTextHandler(io.Discard, nil)), dispatcher, "machine-test").Handler())
	defer server.Close()
	id := fmt.Sprintf("recovery-%d", time.Now().UnixNano())
	body := map[string]any{"dispatchId": id, "tenantId": "tenant-test", "processInstanceId": "process", "nodeInstanceId": "node", "nodeKey": "echo", "executionGeneration": 1, "taskType": "system.echo", "idempotencyKey": id, "input": map[string]any{"message": "recovery"}}
	response := postJSON(t, server.URL+"/dispatches", body)
	response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatal(response.StatusCode)
	}
	deadline := time.Now().Add(15 * time.Second)
	for callbackAttempts.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if callbackAttempts.Load() == 0 {
		agent.Stop()
		service.Stop()
		t.Fatal("callback was never attempted")
	}
	result := awaitAuthenticatedResult(t, server.URL+"/results/"+id)
	if result.Status != "SUCCEEDED" || result.TenantID != "tenant-test" {
		t.Fatalf("result unavailable during callback outage: %+v", result)
	}
	agent.Stop()
	service.Stop()
	if executed.Load() != 1 {
		t.Fatalf("executed %d times before restart", executed.Load())
	}
	accepted.Store(true)
	service, agent = startWorkers()
	defer service.Stop()
	defer agent.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var completed domain.WorkerResult
	if err := temporalClient.GetWorkflow(ctx, id, "").Get(ctx, &completed); err != nil {
		t.Fatal(err)
	}
	if completed.Status != "SUCCEEDED" || deliveries.Load() != 1 || executed.Load() != 1 || callbackAttempts.Load() < 2 {
		t.Fatalf("recovery failed: result=%+v executions=%d deliveries=%d attempts=%d", completed, executed.Load(), deliveries.Load(), callbackAttempts.Load())
	}
	response = postJSON(t, server.URL+"/dispatches", body)
	response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatalf("completed replay %d", response.StatusCode)
	}
	body["input"] = map[string]any{"message": "different"}
	response = postJSON(t, server.URL+"/dispatches", body)
	response.Body.Close()
	if response.StatusCode != 409 {
		t.Fatalf("changed completed replay %d", response.StatusCode)
	}
	if executed.Load() != 1 {
		t.Fatal("completed task executed twice")
	}
	// Check raw JSON payload bytes (not base64-rendered event JSON) so the
	// assertion actually observes credentials if they entered workflow history.
	history := temporalClient.GetWorkflowHistory(ctx, id, "", false, enumspb.HISTORY_EVENT_FILTER_TYPE_ALL_EVENT)
	for history.HasNext() {
		event, err := history.Next()
		if err != nil {
			t.Fatal(err)
		}
		payloads := []*commonpb.Payload{}
		if started := event.GetWorkflowExecutionStartedEventAttributes(); started != nil {
			payloads = append(payloads, started.GetInput().GetPayloads()...)
			for _, p := range started.GetMemo().GetFields() {
				payloads = append(payloads, p)
			}
		}
		if scheduled := event.GetActivityTaskScheduledEventAttributes(); scheduled != nil {
			payloads = append(payloads, scheduled.GetInput().GetPayloads()...)
		}
		if completed := event.GetWorkflowExecutionCompletedEventAttributes(); completed != nil {
			payloads = append(payloads, completed.GetResult().GetPayloads()...)
		}
		for _, p := range payloads {
			for _, secret := range []string{"machine-test", "callback-test"} {
				if bytes.Contains(p.GetData(), []byte(secret)) {
					t.Fatal("machine credential persisted in Temporal history")
				}
			}
		}
	}
	t.Logf("actual Temporal recovery: workflow=%s executions=%d callbackAttempts=%d acceptedDeliveries=%d; workers stopped and restarted", id, executed.Load(), callbackAttempts.Load(), deliveries.Load())
}

type blockingTask struct{ started chan string }

func (b blockingTask) Execute(ctx context.Context, task domain.TaskRequest) (domain.TaskExecutionOutcome, error) {
	b.started <- task.Input["message"].(string)
	<-ctx.Done()
	return domain.TaskExecutionOutcome{}, ctx.Err()
}

func TestRealTemporalCancellationAndTimeout(t *testing.T) {
	if os.Getenv("TEMPORAL_INTEGRATION") != "1" {
		t.Skip("requires explicit isolated real Temporal server")
	}
	host := os.Getenv("TEMPORAL_HOST_PORT")
	if host == "" {
		host = "127.0.0.1:7233"
	}
	temporalClient, err := client.Dial(client.Options{HostPort: host})
	if err != nil {
		t.Fatal(err)
	}
	defer temporalClient.Close()
	callbackResults := make(chan domain.WorkerResult, 4)
	callback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var result domain.WorkerResult
		if err := json.NewDecoder(r.Body).Decode(&result); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		callbackResults <- result
	}))
	defer callback.Close()
	reporter, err := activities.NewCompletionReporter(callback.URL, func(context.Context) (string, error) { return "callback-test", nil }, callback.Client())
	if err != nil {
		t.Fatal(err)
	}
	service := worker.New(temporalClient, queues.ServiceWorkerTaskQueue, worker.Options{WorkerStopTimeout: time.Second})
	service.RegisterWorkflow(workflows.ServiceDispatchWorkflow)
	if err := service.Start(); err != nil {
		t.Fatal(err)
	}
	defer service.Stop()
	started := make(chan string, 4)
	agent := worker.New(temporalClient, queues.AgentExecutorTaskQueue, worker.Options{WorkerStopTimeout: time.Second})
	executor := activities.NewAgentExecutor(blockingTask{started: started})
	agent.RegisterActivityWithOptions(executor.ExecuteTask, activity.RegisterOptions{Name: activities.ExecuteTaskActivityName})
	agent.RegisterActivityWithOptions(reporter.DeliverResult, activity.RegisterOptions{Name: activities.DeliverResultActivityName})
	if err := agent.Start(); err != nil {
		t.Fatal(err)
	}
	defer agent.Stop()
	dispatcher := app.NewTemporalDispatcher(temporalClient)
	server := httptest.NewServer(transporthttp.NewServer(slog.New(slog.NewTextHandler(io.Discard, nil)), dispatcher, "machine-test").Handler())
	defer server.Close()
	for _, status := range []string{"CANCELLED", "TIMEOUT"} {
		t.Run(status, func(t *testing.T) {
			id := fmt.Sprintf("terminal-%s-%d", status, time.Now().UnixNano())
			body := map[string]any{"dispatchId": id, "tenantId": "tenant-test", "processInstanceId": "process", "nodeInstanceId": "node", "nodeKey": "echo", "executionGeneration": 9, "taskType": "system.echo", "idempotencyKey": id, "input": map[string]any{"message": status}, "timeoutSeconds": 2, "retry": map[string]any{"maxAttempts": 1}}
			res := postJSON(t, server.URL+"/dispatches", body)
			res.Body.Close()
			if res.StatusCode != 200 {
				t.Fatal(res.StatusCode)
			}
			select {
			case <-started:
			case <-time.After(10 * time.Second):
				t.Fatal("activity did not start")
			}
			if status == "CANCELLED" {
				res = postJSON(t, server.URL+"/dispatches/"+id+"/cancel", nil)
				res.Body.Close()
				if res.StatusCode != 200 {
					t.Fatal(res.StatusCode)
				}
			}
			select {
			case result := <-callbackResults:
				if result.Status != status || result.TenantID != "tenant-test" || result.ExecutionGeneration != 9 {
					t.Fatalf("wrong terminal callback %+v", result)
				}
			case <-time.After(10 * time.Second):
				t.Fatal("terminal callback did not arrive")
			}
			result := awaitAuthenticatedResult(t, server.URL+"/results/"+id)
			if result.Status != status {
				t.Fatalf("terminal query %+v", result)
			}
			t.Logf("actual Temporal terminal workflow=%s status=%s", id, status)
		})
	}
}

// Baseline 10dda3e command shape, without any version marker or callback.
func scaffoldHistoryFixture(ctx workflow.Context, d domain.DispatchRecord) (domain.WorkerResult, error) {
	ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{TaskQueue: queues.AgentExecutorTaskQueue, StartToCloseTimeout: 30 * time.Second, RetryPolicy: &temporal.RetryPolicy{InitialInterval: time.Second, BackoffCoefficient: 2, MaximumInterval: 10 * time.Second, MaximumAttempts: 3}})
	var outcome domain.TaskExecutionOutcome
	err := workflow.ExecuteActivity(ctx, activities.ExecuteTaskActivityName, d).Get(ctx, &outcome)
	result := domain.WorkerResult{DispatchID: d.DispatchID, ProcessInstanceID: d.ProcessInstanceID, NodeInstanceID: d.NodeInstanceID, NodeKey: d.NodeKey, Status: outcome.Status, Message: outcome.Message, Output: outcome.Output, CompletedAt: workflow.Now(ctx)}
	if result.Status == "" {
		result.Status = "SUCCEEDED"
	}
	if err != nil {
		result.Status = "FAILED"
		result.Message = err.Error()
		result.Output = nil
	}
	return result, nil
}
func TestRealTemporalReplaysScaffoldHistory(t *testing.T) {
	if os.Getenv("TEMPORAL_INTEGRATION") != "1" {
		t.Skip("requires explicit isolated real Temporal server")
	}
	host := os.Getenv("TEMPORAL_HOST_PORT")
	if host == "" {
		host = "127.0.0.1:7233"
	}
	c, err := client.Dial(client.Options{HostPort: host})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	id := fmt.Sprintf("legacy-replay-%d", time.Now().UnixNano())
	service := worker.New(c, id, worker.Options{WorkerStopTimeout: time.Second})
	service.RegisterWorkflowWithOptions(scaffoldHistoryFixture, workflow.RegisterOptions{Name: "ServiceDispatchWorkflow"})
	if err := service.Start(); err != nil {
		t.Fatal(err)
	}
	defer service.Stop()
	agent := worker.New(c, queues.AgentExecutorTaskQueue, worker.Options{WorkerStopTimeout: time.Second})
	agent.RegisterActivityWithOptions(func(context.Context, domain.DispatchRecord) (domain.TaskExecutionOutcome, error) {
		return domain.TaskExecutionOutcome{Status: "SUCCEEDED"}, nil
	}, activity.RegisterOptions{Name: activities.ExecuteTaskActivityName})
	if err := agent.Start(); err != nil {
		t.Fatal(err)
	}
	defer agent.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	run, err := c.ExecuteWorkflow(ctx, client.StartWorkflowOptions{ID: id, TaskQueue: id}, "ServiceDispatchWorkflow", domain.DispatchRecord{DispatchID: id, TaskType: "demo.echo"})
	if err != nil {
		t.Fatal(err)
	}
	if err := run.Get(ctx, nil); err != nil {
		t.Fatal(err)
	}
	history := &historypb.History{}
	iterator := c.GetWorkflowHistory(ctx, id, run.GetRunID(), false, enumspb.HISTORY_EVENT_FILTER_TYPE_ALL_EVENT)
	for iterator.HasNext() {
		event, err := iterator.Next()
		if err != nil {
			t.Fatal(err)
		}
		history.Events = append(history.Events, event)
	}
	replayer := worker.NewWorkflowReplayer()
	replayer.RegisterWorkflow(workflows.ServiceDispatchWorkflow)
	if err := replayer.ReplayWorkflowHistory(nil, history); err != nil {
		t.Fatalf("new worker cannot replay scaffold history: %v", err)
	}
	t.Logf("actual Temporal scaffold history replayed by v2 worker: workflow=%s events=%d", id, len(history.Events))
}
