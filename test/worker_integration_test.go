package test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aurora/aurora-helix-go-execution-engine/internal/domain"
	transporthttp "github.com/aurora/aurora-helix-go-execution-engine/internal/transport/http"
)

func TestDispatchHappyPath(t *testing.T) {
	fake := &fakeDispatcher{
		dispatchResult: domain.WorkerResult{
			ResultID:          "dispatch-1:result",
			DispatchID:        "dispatch-1",
			ProcessInstanceID: "proc-001",
			NodeKey:           "serviceNode",
			ResultStatus:      "PLACEHOLDER_SUCCESS",
			ResultMessage:     "demo task executed",
			Output: map[string]any{
				"echoedMessage": "hello worker",
			},
		},
	}
	server := newTestServer(fake)
	reqBody := map[string]any{
		"processInstanceId": "proc-001",
		"nodeKey":           "serviceNode",
		"payload": map[string]any{
			"taskType": "demo.echo",
			"input": map[string]any{
				"message": "hello worker",
			},
		},
	}
	body, _ := json.Marshal(reqBody)
	resp, err := http.Post(server.URL+"/dispatches", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("dispatch request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(payload))
	}
	var result struct {
		Dispatch struct {
			DispatchID        string `json:"dispatchId"`
			ProcessInstanceID string `json:"processInstanceId"`
			NodeKey           string `json:"nodeKey"`
		} `json:"dispatch"`
		Result struct {
			DispatchID    string         `json:"dispatchId"`
			ResultStatus  string         `json:"resultStatus"`
			ResultMessage string         `json:"resultMessage"`
			Output        map[string]any `json:"output"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Result.ResultStatus != "PLACEHOLDER_SUCCESS" {
		t.Fatalf("expected success result, got %s", result.Result.ResultStatus)
	}
	if result.Dispatch.ProcessInstanceID != "proc-001" || result.Dispatch.NodeKey != "serviceNode" {
		t.Fatalf("unexpected dispatch fields: %+v", result.Dispatch)
	}
	if result.Result.Output["echoedMessage"] != "hello worker" {
		t.Fatalf("unexpected output: %+v", result.Result.Output)
	}

	fake.getResult = domain.WorkerResult{DispatchID: result.Dispatch.DispatchID, ResultStatus: "PLACEHOLDER_SUCCESS"}
	resultResp, err := http.Get(server.URL + "/results/" + result.Dispatch.DispatchID)
	if err != nil {
		t.Fatalf("get result failed: %v", err)
	}
	defer resultResp.Body.Close()
	if resultResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from result lookup, got %d", resultResp.StatusCode)
	}
}

func TestDispatchUnsupportedTaskType(t *testing.T) {
	fake := &fakeDispatcher{
		dispatchResult: domain.WorkerResult{ResultStatus: "FAILED", ResultMessage: "unsupported taskType: demo.unknown"},
	}
	server := newTestServer(fake)
	reqBody := map[string]any{
		"processInstanceId": "proc-unsupported",
		"nodeKey":           "serviceNode",
		"payload": map[string]any{
			"taskType": "demo.unknown",
			"input": map[string]any{
				"message": "hello worker",
			},
		},
	}
	body, _ := json.Marshal(reqBody)
	resp, err := http.Post(server.URL+"/dispatches", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("dispatch request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var result struct {
		Result struct {
			ResultStatus  string `json:"resultStatus"`
			ResultMessage string `json:"resultMessage"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Result.ResultStatus != "FAILED" {
		t.Fatalf("expected FAILED result, got %s", result.Result.ResultStatus)
	}
}

func TestResultLookupFailure(t *testing.T) {
	server := newTestServer(&fakeDispatcher{getResultErr: errors.New("result not found")})
	resp, err := http.Get(server.URL + "/results/missing")
	if err != nil {
		t.Fatalf("get result failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func newTestServer(dispatcher *fakeDispatcher) *httptest.Server {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := transporthttp.NewServer(logger, dispatcher)
	return httptest.NewServer(server.Handler())
}

type fakeDispatcher struct {
	dispatchResult domain.WorkerResult
	dispatchErr    error
	getResult      domain.WorkerResult
	getResultErr   error
}

func (f *fakeDispatcher) Dispatch(_ context.Context, dispatch domain.DispatchRecord) (domain.WorkerResult, error) {
	result := f.dispatchResult
	if result.DispatchID == "" {
		result.DispatchID = dispatch.DispatchID
	}
	if result.ProcessInstanceID == "" {
		result.ProcessInstanceID = dispatch.ProcessInstanceID
	}
	if result.NodeKey == "" {
		result.NodeKey = dispatch.NodeKey
	}
	return result, f.dispatchErr
}

func (f *fakeDispatcher) GetResult(_ context.Context, _ string) (domain.WorkerResult, error) {
	if f.getResultErr != nil {
		return domain.WorkerResult{}, f.getResultErr
	}
	return f.getResult, nil
}
