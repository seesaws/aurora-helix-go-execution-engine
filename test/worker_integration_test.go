package test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aurora/aurora-helix-go-execution-engine/internal/app"
	"github.com/aurora/aurora-helix-go-execution-engine/internal/domain"
	"github.com/aurora/aurora-helix-go-execution-engine/internal/executor/demo"
	transporthttp "github.com/aurora/aurora-helix-go-execution-engine/internal/transport/http"
)

func TestDispatchContractHappyPath(t *testing.T) {
	server := newTestServer()
	reqBody := map[string]any{
		"dispatchId":        "sop-123-node-456-attempt-1",
		"processInstanceId": "sop_instance_123",
		"nodeInstanceId":    "node_instance_456",
		"nodeKey":           "service_create_rfq",
		"taskType":          "demo.echo",
		"idempotencyKey":    "sop_instance_123:service_create_rfq:1",
		"input": map[string]any{
			"message": "hello worker",
		},
		"metadata": map[string]any{
			"tenantId": "tenant_001",
			"traceId":  "trace_001",
		},
		"retry": map[string]any{
			"maxAttempts": 3,
			"backoff":     "PT10S",
		},
		"timeoutSeconds": 30,
	}
	resp := postJSON(t, server.URL+"/dispatches", reqBody)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(payload))
	}
	var ack struct {
		DispatchID string `json:"dispatchId"`
		Status     string `json:"status"`
		Dispatch   any    `json:"dispatch"`
		Result     any    `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&ack); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if ack.DispatchID != "sop-123-node-456-attempt-1" || ack.Status != domain.DispatchStatusDispatched {
		t.Fatalf("unexpected dispatch ack: %+v", ack)
	}
	if ack.Dispatch != nil || ack.Result != nil {
		t.Fatalf("dispatch response must not include nested dispatch/result placeholders: %+v", ack)
	}

	result := awaitResult(t, server.URL+"/results/"+ack.DispatchID)
	if result.Status != domain.ResultStatusSucceeded {
		t.Fatalf("expected SUCCEEDED result, got %+v", result)
	}
	if result.ProcessInstanceID != "sop_instance_123" || result.NodeInstanceID != "node_instance_456" || result.NodeKey != "service_create_rfq" {
		t.Fatalf("unexpected result identity fields: %+v", result)
	}
	if result.Output["echoedMessage"] != "hello worker" {
		t.Fatalf("unexpected output: %+v", result.Output)
	}
}

func TestDispatchSupplychainRFQCreate(t *testing.T) {
	server := newTestServer()
	reqBody := map[string]any{
		"dispatchId":        "rfq-dispatch-1",
		"processInstanceId": "sop_instance_123",
		"nodeInstanceId":    "node_instance_456",
		"nodeKey":           "service_create_rfq",
		"taskType":          "supplychain.rfq.create",
		"input": map[string]any{
			"salesOrderId": "SO-001",
		},
	}
	resp := postJSON(t, server.URL+"/dispatches", reqBody)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(payload))
	}
	result := awaitResult(t, server.URL+"/results/rfq-dispatch-1")
	if result.Status != domain.ResultStatusSucceeded || result.Message != "created rfq" {
		t.Fatalf("expected RFQ success, got %+v", result)
	}
	if result.Output["rfqId"] != "RFQ-SO-001" {
		t.Fatalf("expected rfqId output, got %+v", result.Output)
	}
}

func TestDispatchUnsupportedTaskType(t *testing.T) {
	server := newTestServer()
	reqBody := map[string]any{
		"dispatchId":        "unsupported-dispatch-1",
		"processInstanceId": "proc-unsupported",
		"nodeInstanceId":    "node-unsupported",
		"nodeKey":           "serviceNode",
		"taskType":          "demo.unknown",
		"input": map[string]any{
			"message": "hello worker",
		},
	}
	resp := postJSON(t, server.URL+"/dispatches", reqBody)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	result := awaitResult(t, server.URL+"/results/unsupported-dispatch-1")
	if result.Status != domain.ResultStatusFailed {
		t.Fatalf("expected FAILED result, got %+v", result)
	}
}

func TestDispatchValidationRequiresTopLevelFields(t *testing.T) {
	server := newTestServer()
	resp := postJSON(t, server.URL+"/dispatches", map[string]any{
		"processInstanceId": "proc-001",
		"nodeKey":           "serviceNode",
		"payload": map[string]any{
			"taskType": "demo.echo",
		},
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestResultLookupFailure(t *testing.T) {
	server := newTestServer()
	resp, err := http.Get(server.URL + "/results/missing")
	if err != nil {
		t.Fatalf("get result failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func newTestServer() *httptest.Server {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := transporthttp.NewServer(logger, app.NewMemoryDispatcher(demo.NewRegistry()))
	return httptest.NewServer(server.Handler())
}

func postJSON(t *testing.T, url string, body map[string]any) *http.Response {
	t.Helper()
	payload, _ := json.Marshal(body)
	resp, err := http.Post(url, "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("post %s failed: %v", url, err)
	}
	return resp
}

func awaitResult(t *testing.T, url string) domain.WorkerResult {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err != nil {
			t.Fatalf("get result failed: %v", err)
		}
		var result domain.WorkerResult
		if resp.StatusCode == http.StatusOK {
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				resp.Body.Close()
				t.Fatalf("decode result: %v", err)
			}
			resp.Body.Close()
			if result.Status != domain.ResultStatusRunning {
				return result
			}
		} else {
			resp.Body.Close()
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for result at %s", url)
	return domain.WorkerResult{}
}
