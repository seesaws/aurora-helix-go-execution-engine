package app_test

import (
	"context"
	"errors"
	"testing"

	"github.com/aurora/aurora-helix-go-execution-engine/internal/app"
	"github.com/aurora/aurora-helix-go-execution-engine/internal/domain"
	"github.com/stretchr/testify/mock"
	commonpb "go.temporal.io/api/common/v1"
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
	workflowpb "go.temporal.io/api/workflow/v1"
	workflowservice "go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/converter"
	"go.temporal.io/sdk/mocks"
)

func TestCompletedWorkflowIDCannotRestartAndChangedInputConflicts(t *testing.T) {
	ctx := context.Background()
	c := &mocks.Client{}
	d := app.NewTemporalDispatcher(c)
	original := domain.DispatchRecord{DispatchID: "dispatch", TenantID: "tenant", ExecutionGeneration: 1, ProcessInstanceID: "process", NodeInstanceID: "node", NodeKey: "echo", TaskType: "system.echo", IdempotencyKey: "key", Input: map[string]any{"message": "hello"}}
	hash, _ := domain.SemanticHash(original)
	payload, _ := converter.GetDefaultDataConverter().ToPayload(hash)
	c.On("ExecuteWorkflow", mock.Anything, mock.MatchedBy(func(o client.StartWorkflowOptions) bool {
		return o.WorkflowExecutionErrorWhenAlreadyStarted && o.WorkflowIDReusePolicy == enumspb.WORKFLOW_ID_REUSE_POLICY_REJECT_DUPLICATE
	}), mock.Anything, mock.Anything).Return(nil, &serviceerror.WorkflowExecutionAlreadyStarted{}).Twice()
	c.On("DescribeWorkflowExecution", mock.Anything, "dispatch", "").Return(&workflowservice.DescribeWorkflowExecutionResponse{WorkflowExecutionInfo: &workflowpb.WorkflowExecutionInfo{Status: enumspb.WORKFLOW_EXECUTION_STATUS_COMPLETED, Memo: &commonpb.Memo{Fields: map[string]*commonpb.Payload{"semanticHash": payload}}}}, nil).Twice()
	if _, err := d.Dispatch(ctx, original); err != nil {
		t.Fatal(err)
	}
	original.Input = map[string]any{"message": "changed"}
	if _, err := d.Dispatch(ctx, original); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
	c.AssertExpectations(t)
}
