package test

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/aurora/aurora-helix-go-execution-engine/internal/domain"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aurora/aurora-helix-go-execution-engine/internal/app"
	"github.com/aurora/aurora-helix-go-execution-engine/internal/executor/system"
	transporthttp "github.com/aurora/aurora-helix-go-execution-engine/internal/transport/http"
)

func TestMachineCredentialRequiredOnEveryProtocolRoute(t *testing.T) {
	server := httptest.NewServer(transporthttp.NewServer(slog.New(slog.NewTextHandler(io.Discard, nil)), app.NewMemoryDispatcher(system.Executor{}), "machine-test").Handler())
	defer server.Close()
	for _, route := range []struct{ method, path string }{{"POST", "/dispatches"}, {"GET", "/results/id"}, {"POST", "/dispatches/id/cancel"}} {
		for _, token := range []string{"", "Bearer human-token", "Bearer machine-test"} {
			req, _ := http.NewRequest(route.method, server.URL+route.path, bytes.NewBufferString(`{}`))
			req.Header.Set("Authorization", token)
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			res.Body.Close()
			if token != "Bearer machine-test" && res.StatusCode != 401 {
				t.Fatalf("%s %s expected unauthorized, got %d", route.method, route.path, res.StatusCode)
			}
			if token == "Bearer machine-test" && res.StatusCode == 401 {
				t.Fatal("machine credential rejected")
			}
		}
	}
}

func TestCompletedDispatchSemanticReplayAndConflict(t *testing.T) {
	server := httptest.NewServer(transporthttp.NewServer(slog.New(slog.NewTextHandler(io.Discard, nil)), app.NewMemoryDispatcher(system.Executor{}), "machine-test").Handler())
	defer server.Close()
	body := `{"dispatchId":"same-id","tenantId":"tenant-1","processInstanceId":"process-1","nodeInstanceId":"node-1","nodeKey":"echo","executionGeneration":1,"taskType":"system.echo","idempotencyKey":"stable-key","input":{"message":"hello","amount":1.0}}`
	send := func(payload string) int {
		req, _ := http.NewRequest("POST", server.URL+"/dispatches", bytes.NewBufferString(payload))
		req.Header.Set("Authorization", "Bearer machine-test")
		res, e := http.DefaultClient.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer res.Body.Close()
		return res.StatusCode
	}
	if code := send(body); code != 200 {
		t.Fatalf("initial dispatch got %d", code)
	}
	result := awaitAuthenticatedResult(t, server.URL+"/results/same-id")
	if result.TenantID != "tenant-1" || result.ExecutionGeneration != 1 || result.Status != "SUCCEEDED" {
		t.Fatalf("wrong envelope %+v", result)
	}
	equivalent := `{"nodeKey":"echo","nodeInstanceId":"node-1","processInstanceId":"process-1","tenantId":"tenant-1","dispatchId":"same-id","executionGeneration":1,"taskType":"system.echo","idempotencyKey":"stable-key","input":{"amount":1,"message":"hello"},"timeoutSeconds":30,"retry":{"maxAttempts":3,"backoff":"PT1S"}}`
	if code := send(equivalent); code != 200 {
		t.Fatalf("semantic replay got %d", code)
	}
	if code := send(bytesToChangedInput(body)); code != 409 {
		t.Fatalf("changed replay got %d", code)
	}
}
func bytesToChangedInput(body string) string {
	var v map[string]any
	_ = json.Unmarshal([]byte(body), &v)
	v["input"] = map[string]any{"message": "changed"}
	b, _ := json.Marshal(v)
	return string(b)
}
func awaitAuthenticatedResult(t *testing.T, url string) domain.WorkerResult {
	t.Helper()
	for i := 0; i < 500; i++ {
		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Set("Authorization", "Bearer machine-test")
		res, e := http.DefaultClient.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		var result domain.WorkerResult
		_ = json.NewDecoder(res.Body).Decode(&result)
		res.Body.Close()
		if res.StatusCode == http.StatusOK && domain.IsTerminal(result.Status) {
			return result
		}
		time.Sleep(time.Millisecond * 10)
	}
	t.Fatal("result timeout")
	return domain.WorkerResult{}
}

type lateExecutor struct {
	release  chan struct{}
	finished chan struct{}
}

func (e lateExecutor) Execute(context.Context, domain.TaskRequest) (domain.TaskExecutionOutcome, error) {
	<-e.release
	defer close(e.finished)
	return domain.TaskExecutionOutcome{Status: "SUCCEEDED"}, nil
}
func TestCancelFencesLateCompletionAndReplaysOriginalIdentity(t *testing.T) {
	executor := lateExecutor{release: make(chan struct{}), finished: make(chan struct{})}
	server := httptest.NewServer(transporthttp.NewServer(slog.New(slog.NewTextHandler(io.Discard, nil)), app.NewMemoryDispatcher(executor), "machine-test").Handler())
	defer server.Close()
	body := map[string]any{"dispatchId": "cancel", "tenantId": "tenant-1", "processInstanceId": "process", "nodeInstanceId": "node", "nodeKey": "echo", "executionGeneration": 2, "taskType": "system.echo", "idempotencyKey": "key"}
	res := postJSON(t, server.URL+"/dispatches", body)
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatal(res.StatusCode)
	}
	res = postJSON(t, server.URL+"/dispatches/cancel/cancel", map[string]any{})
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatal(res.StatusCode)
	}
	close(executor.release)
	<-executor.finished
	result := awaitAuthenticatedResult(t, server.URL+"/results/cancel")
	if result.Status != "CANCELLED" || result.TenantID != "tenant-1" || result.ExecutionGeneration != 2 {
		t.Fatalf("cancel fence failed: %+v", result)
	}
	res = postJSON(t, server.URL+"/dispatches", body)
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatal(res.StatusCode)
	}
	body["executionGeneration"] = 3
	res = postJSON(t, server.URL+"/dispatches", body)
	res.Body.Close()
	if res.StatusCode != 409 {
		t.Fatal(res.StatusCode)
	}
}

func TestProtocolRejectsUntrustedOrUnboundedPayload(t *testing.T) {
	server := newTestServer()
	defer server.Close()
	valid := map[string]any{"dispatchId": "validation", "tenantId": "tenant", "processInstanceId": "process", "nodeInstanceId": "node", "nodeKey": "echo", "executionGeneration": 1, "taskType": "system.echo", "idempotencyKey": "key"}
	cases := []map[string]any{
		{"metadata": map[string]any{"callbackUrl": "http://untrusted"}},
		{"metadata": map[string]any{"authorization": "sensitive"}},
		{"input": map[string]any{"token": "sensitive"}},
		{"input": map[string]any{"amount": json.Number("1e999999999")}},
		{"timeoutSeconds": -1}, {"retry": map[string]any{"maxAttempts": 11}},
		{"retry": map[string]any{"backoff": "forever"}}, {"executionGeneration": 0}, {"tenantId": ""}, {"dispatchId": "../escape"},
		{"dispatchId": ".."},
		{"unknownField": true},
	}
	for _, overrides := range cases {
		body := map[string]any{}
		for key, value := range valid {
			body[key] = value
		}
		for key, value := range overrides {
			body[key] = value
		}
		res := postJSON(t, server.URL+"/dispatches", body)
		res.Body.Close()
		if res.StatusCode != 400 {
			t.Fatalf("invalid payload admitted: fields %v status %d", overrides, res.StatusCode)
		}
	}
}
