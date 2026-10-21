package test

import (
	"context"
	"encoding/json"
	"github.com/aurora/aurora-helix-go-execution-engine/internal/domain"
	"github.com/aurora/aurora-helix-go-execution-engine/internal/temporal/activities"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCompletionDeliveryUsesFixedTargetAndRuntimeCredential(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/internal/sop/service-dispatches/dispatch-1/result" {
			t.Errorf("wrong callback path %s", r.URL.Path)
		}
		expected := "Bearer first"
		if calls == 2 {
			expected = "Bearer rotated"
		}
		if r.Header.Get("Authorization") != expected {
			t.Error("runtime credential not used")
		}
		var result domain.WorkerResult
		if err := json.NewDecoder(r.Body).Decode(&result); err != nil {
			t.Error(err)
		}
		if result.TenantID != "tenant" || result.ExecutionGeneration != 4 {
			t.Errorf("wrong identity %+v", result)
		}
		if calls == 1 {
			w.WriteHeader(503)
		}
	}))
	defer server.Close()
	token := "first"
	reporter, err := activities.NewCompletionReporter(server.URL, func(context.Context) (string, error) { return token, nil }, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	result := domain.WorkerResult{DispatchID: "dispatch-1", TenantID: "tenant", ExecutionGeneration: 4, Status: "SUCCEEDED"}
	if err := reporter.DeliverResult(context.Background(), result); err == nil {
		t.Fatal("callback outage swallowed")
	}
	token = "rotated"
	if err := reporter.DeliverResult(context.Background(), result); err != nil {
		t.Fatal(err)
	}
}

func TestCallbackDoesNotFollowRedirectOrExposeCredentialErrors(t *testing.T) {
	leaked := false
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { leaked = true }))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 307) }))
	defer source.Close()
	reporter, err := activities.NewCompletionReporter(source.URL, func(context.Context) (string, error) { return "private-service-token", nil }, source.Client())
	if err != nil {
		t.Fatal(err)
	}
	if err := reporter.DeliverResult(context.Background(), domain.WorkerResult{DispatchID: "id", Status: "SUCCEEDED"}); err == nil {
		t.Fatal("redirect accepted")
	}
	if leaked {
		t.Fatal("credential forwarded to redirected target")
	}
}

func TestCompletionDeliveryUsesServerConfiguredContextPrefix(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sop/internal/sop/service-dispatches/id/result" {
			t.Errorf("wrong configured context path %s", r.URL.Path)
		}
	}))
	defer server.Close()
	reporter, err := activities.NewCompletionReporter(server.URL+"/sop", func(context.Context) (string, error) { return "machine-test", nil }, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if err := reporter.DeliverResult(context.Background(), domain.WorkerResult{DispatchID: "id", Status: "SUCCEEDED"}); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"/sop/../other", "/sop/%2e%2e/other", "//other", "/sop?query=1", "/sop#fragment"} {
		if _, err := activities.NewCompletionReporter(server.URL+suffix, func(context.Context) (string, error) { return "machine-test", nil }, nil); err == nil {
			t.Fatalf("unsafe configured prefix accepted: %s", suffix)
		}
	}
}
