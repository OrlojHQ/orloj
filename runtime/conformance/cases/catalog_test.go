package cases

import (
	"testing"

	agentruntime "github.com/AnonJon/orloj/runtime"
)

func TestBaseRequestDefaults(t *testing.T) {
	req := BaseRequest("req-1", "web_search")
	if req.ToolContractVersion != agentruntime.ToolContractVersionV1 {
		t.Fatalf("expected version=%s got %s", agentruntime.ToolContractVersionV1, req.ToolContractVersion)
	}
	if req.RequestID != "req-1" {
		t.Fatalf("unexpected request id %q", req.RequestID)
	}
	if req.Tool.Name != "web_search" {
		t.Fatalf("unexpected tool name %q", req.Tool.Name)
	}
	if req.Tool.Operation != agentruntime.ToolOperationInvoke {
		t.Fatalf("unexpected operation %q", req.Tool.Operation)
	}
	if req.Attempt != 1 {
		t.Fatalf("expected attempt=1 got %d", req.Attempt)
	}
}

func TestUnknownVersionCase(t *testing.T) {
	item := UnknownVersionCase("req-2", "web_search")
	if item.Request.ToolContractVersion != "v9" {
		t.Fatalf("expected request version v9, got %q", item.Request.ToolContractVersion)
	}
	if item.Expected.Status != agentruntime.ToolExecutionStatusError {
		t.Fatalf("expected status=%s got %s", agentruntime.ToolExecutionStatusError, item.Expected.Status)
	}
	if item.Expected.ErrorCode != agentruntime.ToolCodeInvalidInput {
		t.Fatalf("expected error code=%s got %s", agentruntime.ToolCodeInvalidInput, item.Expected.ErrorCode)
	}
}
