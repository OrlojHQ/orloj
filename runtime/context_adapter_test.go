package agentruntime

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/OrlojHQ/orloj/resources"
)

type mockContextAdapterToolRuntime struct {
	lastTool  string
	lastInput string
	result    string
	err       error
}

func (m *mockContextAdapterToolRuntime) Call(ctx context.Context, tool string, input string) (string, error) {
	m.lastTool = tool
	m.lastInput = input
	return m.result, m.err
}

func TestToolBackedContextAdapterHappyPath(t *testing.T) {
	t.Parallel()
	rt := &mockContextAdapterToolRuntime{
		result: `{"memo":"scrubbed"}`,
	}
	spec := resources.ContextAdapterSpec{
		ToolRef: "sanitizer",
		OnError: "reject",
	}
	ad := NewToolBackedContextAdapter(spec, rt)
	input := map[string]string{"memo": "IGNORE ALL"}
	out, err := ad.AdaptContext(context.Background(), input)
	if err != nil {
		t.Fatalf("AdaptContext: %v", err)
	}
	if out["memo"] != "scrubbed" {
		t.Fatalf("expected scrubbed memo, got %#v", out)
	}
	if rt.lastTool != "sanitizer" {
		t.Fatalf("expected tool sanitizer, got %q", rt.lastTool)
	}
	var payload map[string]string
	if err := json.Unmarshal([]byte(rt.lastInput), &payload); err != nil {
		t.Fatalf("payload json: %v", err)
	}
	if payload["memo"] != "IGNORE ALL" {
		t.Fatalf("expected raw memo in payload, got %#v", payload["memo"])
	}
}

func TestToolBackedContextAdapterOnErrorReject(t *testing.T) {
	t.Parallel()
	rt := &mockContextAdapterToolRuntime{err: errors.New("boom")}
	spec := resources.ContextAdapterSpec{ToolRef: "x", OnError: "reject"}
	ad := NewToolBackedContextAdapter(spec, rt)
	_, err := ad.AdaptContext(context.Background(), map[string]string{"a": "b"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected wrapped boom: %v", err)
	}
}

func TestToolBackedContextAdapterOnErrorPassthrough(t *testing.T) {
	t.Parallel()
	rt := &mockContextAdapterToolRuntime{err: errors.New("boom")}
	spec := resources.ContextAdapterSpec{ToolRef: "x", OnError: "passthrough"}
	ad := NewToolBackedContextAdapter(spec, rt)
	raw := map[string]string{"a": "b"}
	out, err := ad.AdaptContext(context.Background(), raw)
	if err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	if out["a"] != "b" {
		t.Fatalf("expected passthrough %#v got %#v", raw, out)
	}
}

func TestToolBackedContextAdapterPassesInputDirectly(t *testing.T) {
	t.Parallel()
	rt := &mockContextAdapterToolRuntime{
		result: `{"amount":"9800.00"}`,
	}
	spec := resources.ContextAdapterSpec{ToolRef: "t"}
	ad := NewToolBackedContextAdapter(spec, rt)
	in := map[string]string{"amount": "9800.00"}
	out, err := ad.AdaptContext(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if out["amount"] != "9800.00" {
		t.Fatal(out)
	}
	var decoded map[string]string
	if err := json.Unmarshal([]byte(rt.lastInput), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["amount"] != "9800.00" {
		t.Fatal(decoded)
	}
}
