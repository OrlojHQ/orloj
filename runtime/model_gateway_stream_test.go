package agentruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/OrlojHQ/orloj/resources"
)

func TestReadServerSentEventsHandlesFraming(t *testing.T) {
	input := ": keepalive\r\n" +
		"event: custom\r\n" +
		"data: {\"value\":\r\n" +
		"data: 1}\r\n\r\n" +
		"data: final"
	var events []serverSentEvent
	err := readServerSentEvents(strings.NewReader(input), func(event serverSentEvent) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatalf("read SSE: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Type != "custom" || events[0].Data != "{\"value\":\n1}" {
		t.Fatalf("unexpected multiline event: %+v", events[0])
	}
	if events[1].Data != "final" {
		t.Fatalf("expected final EOF-delimited event, got %+v", events[1])
	}
}

func TestStreamingHTTPClientRemovesOnlyTotalTimeout(t *testing.T) {
	transport := http.DefaultTransport
	client := &http.Client{Timeout: 30 * time.Second, Transport: transport}
	streaming := streamingHTTPClient(client)
	if streaming == client {
		t.Fatal("expected timed client to be cloned")
	}
	if streaming.Timeout != 0 {
		t.Fatalf("streaming timeout = %s, want 0", streaming.Timeout)
	}
	if streaming.Transport != transport {
		t.Fatal("streaming client did not preserve transport")
	}
	if client.Timeout != 30*time.Second {
		t.Fatalf("source timeout changed to %s", client.Timeout)
	}
}

func TestReadServerSentEventsAcceptsLinesLargerThanScannerDefault(t *testing.T) {
	payload := strings.Repeat("x", 128*1024)
	var got string
	err := readServerSentEvents(strings.NewReader("data: "+payload+"\n\n"), func(event serverSentEvent) error {
		got = event.Data
		return nil
	})
	if err != nil {
		t.Fatalf("read large SSE event: %v", err)
	}
	if got != payload {
		t.Fatalf("large SSE payload length=%d, want %d", len(got), len(payload))
	}
}

func TestOpenAIModelGatewayStreamTextToolsUsage(t *testing.T) {
	var captured struct {
		Stream        bool `json:"stream"`
		StreamOptions struct {
			IncludeUsage bool `json:"include_usage"`
		} `json:"stream_options"`
	}
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			body, err := io.ReadAll(req.Body)
			if err != nil {
				return nil, err
			}
			if err := json.Unmarshal(body, &captured); err != nil {
				return nil, err
			}
			stream := strings.Join([]string{
				": keepalive\r\n\r\n",
				"data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"},\"finish_reason\":null}]}\r\n\r\n",
				"data: {\"choices\":[{\"delta\":{\"content\":\" world\",\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"function\":{\"name\":\"memory_\",\"arguments\":\"{\\\"input\\\":\\\"he\"}}]},\"finish_reason\":null}]}\r\n\r\n",
				"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"name\":\"write\",\"arguments\":\"llo\\\"}\"}}]},\"finish_reason\":\"tool_calls\"}]}\r\n\r\n",
				"data: {\"choices\":[],\"usage\":{\"prompt_tokens\":8,\"completion_tokens\":4,\"total_tokens\":12}}\r\n\r\n",
				"data: [DONE]\r\n\r\n",
			}, "")
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
				Body:       io.NopCloser(strings.NewReader(stream)),
			}, nil
		}),
		Timeout: time.Second,
	}
	cfg := DefaultOpenAIModelGatewayConfig()
	cfg.APIKey = "test-key"
	cfg.BaseURL = "https://example.invalid/v1"
	cfg.HTTPClient = client
	gateway, err := NewOpenAIModelGateway(cfg)
	if err != nil {
		t.Fatalf("new gateway: %v", err)
	}

	var events []ModelStreamEvent
	resp, err := gateway.Stream(context.Background(), ModelRequest{
		Model: "gpt-test",
		Tools: []string{"memory.write"},
	}, func(event ModelStreamEvent) {
		events = append(events, event)
	})
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	if !captured.Stream || !captured.StreamOptions.IncludeUsage {
		t.Fatalf("expected streaming request with usage, got %+v", captured)
	}
	if resp.Content != "Hello world" {
		t.Fatalf("unexpected content %q", resp.Content)
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Name != "memory.write" || resp.ToolCalls[0].Input != "hello" {
		t.Fatalf("unexpected tool calls: %+v", resp.ToolCalls)
	}
	if resp.Usage.TotalTokens != 12 {
		t.Fatalf("unexpected usage: %+v", resp.Usage)
	}
	assertModelStreamEventTypes(t, events,
		ModelStreamEventTextDelta,
		ModelStreamEventTextDelta,
		ModelStreamEventToolCall,
		ModelStreamEventUsage,
		ModelStreamEventCompletion,
	)
}

func TestOpenAIModelGatewayStreamRejectsTruncatedStream(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body: io.NopCloser(strings.NewReader(
				"data: {\"choices\":[{\"delta\":{\"content\":\"partial\"},\"finish_reason\":null}]}\n\n",
			)),
		}, nil
	})}
	cfg := DefaultOpenAIModelGatewayConfig()
	cfg.APIKey = "test-key"
	cfg.HTTPClient = client
	gateway, err := NewOpenAIModelGateway(cfg)
	if err != nil {
		t.Fatalf("new gateway: %v", err)
	}
	var events []ModelStreamEvent
	_, err = gateway.Stream(context.Background(), ModelRequest{Model: "gpt-test"}, func(event ModelStreamEvent) {
		events = append(events, event)
	})
	if err == nil || !strings.Contains(err.Error(), "before completion") {
		t.Fatalf("expected truncated stream error, got %v", err)
	}
	assertModelStreamEventTypes(t, events, ModelStreamEventTextDelta, ModelStreamEventError)
}

func TestAnthropicModelGatewayStreamTextToolsUsage(t *testing.T) {
	var captured struct {
		Stream bool `json:"stream"`
	}
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			body, err := io.ReadAll(req.Body)
			if err != nil {
				return nil, err
			}
			if err := json.Unmarshal(body, &captured); err != nil {
				return nil, err
			}
			stream := strings.Join([]string{
				"event: message_start\n",
				"data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":7,\"output_tokens\":0,\"cache_read_input_tokens\":2}}}\n\n",
				"event: content_block_start\n",
				"data: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n",
				"event: content_block_delta\n",
				"data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Hello\"}}\n\n",
				"event: content_block_delta\n",
				"data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\" world\"}}\n\n",
				"event: content_block_stop\n",
				"data: {\"type\":\"content_block_stop\",\"index\":0}\n\n",
				"event: content_block_start\n",
				"data: {\"type\":\"content_block_start\",\"index\":1,\"content_block\":{\"type\":\"tool_use\",\"id\":\"toolu_1\",\"name\":\"memory_write\",\"input\":{}}}\n\n",
				"event: content_block_delta\n",
				"data: {\"type\":\"content_block_delta\",\"index\":1,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"{\\\"input\\\":\\\"note\"}}\n\n",
				"event: content_block_delta\n",
				"data: {\"type\":\"content_block_delta\",\"index\":1,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"\\\"}\"}}\n\n",
				"event: content_block_stop\n",
				"data: {\"type\":\"content_block_stop\",\"index\":1}\n\n",
				"event: message_delta\n",
				"data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"tool_use\"},\"usage\":{\"output_tokens\":5}}\n\n",
				"event: message_stop\n",
				"data: {\"type\":\"message_stop\"}",
			}, "")
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
				Body:       io.NopCloser(strings.NewReader(stream)),
			}, nil
		}),
		Timeout: time.Second,
	}
	cfg := DefaultAnthropicModelGatewayConfig()
	cfg.APIKey = "test-key"
	cfg.BaseURL = "https://example.invalid/v1"
	cfg.HTTPClient = client
	gateway, err := NewAnthropicModelGateway(cfg)
	if err != nil {
		t.Fatalf("new gateway: %v", err)
	}

	var events []ModelStreamEvent
	resp, err := gateway.Stream(context.Background(), ModelRequest{
		Model: "claude-test",
		Tools: []string{"memory.write"},
	}, func(event ModelStreamEvent) {
		events = append(events, event)
	})
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	if !captured.Stream {
		t.Fatal("expected stream=true in Anthropic request")
	}
	if resp.Content != "Hello world" {
		t.Fatalf("unexpected content %q", resp.Content)
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Name != "memory.write" || resp.ToolCalls[0].Input != "note" {
		t.Fatalf("unexpected tool calls: %+v", resp.ToolCalls)
	}
	if resp.Usage.InputTokens != 9 || resp.Usage.OutputTokens != 5 || resp.Usage.TotalTokens != 14 {
		t.Fatalf("unexpected usage: %+v", resp.Usage)
	}
	assertModelStreamEventTypes(t, events,
		ModelStreamEventUsage,
		ModelStreamEventTextDelta,
		ModelStreamEventTextDelta,
		ModelStreamEventToolCall,
		ModelStreamEventUsage,
		ModelStreamEventCompletion,
	)
}

func TestAnthropicModelGatewayStreamEmitsProviderError(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body: io.NopCloser(strings.NewReader(
				"event: error\ndata: {\"type\":\"error\",\"error\":{\"message\":\"overloaded\"}}\n\n",
			)),
		}, nil
	})}
	cfg := DefaultAnthropicModelGatewayConfig()
	cfg.APIKey = "test-key"
	cfg.HTTPClient = client
	gateway, err := NewAnthropicModelGateway(cfg)
	if err != nil {
		t.Fatalf("new gateway: %v", err)
	}
	var events []ModelStreamEvent
	_, err = gateway.Stream(context.Background(), ModelRequest{Model: "claude-test"}, func(event ModelStreamEvent) {
		events = append(events, event)
	})
	if err == nil || !strings.Contains(err.Error(), "overloaded") {
		t.Fatalf("expected provider stream error, got %v", err)
	}
	assertModelStreamEventTypes(t, events, ModelStreamEventError)
}

type scriptedStreamingGateway struct {
	mu       sync.Mutex
	calls    int
	events   []ModelStreamEvent
	response ModelResponse
	err      error
}

func (g *scriptedStreamingGateway) Complete(context.Context, ModelRequest) (ModelResponse, error) {
	return ModelResponse{}, fmt.Errorf("unexpected blocking completion")
}

func (g *scriptedStreamingGateway) Stream(_ context.Context, _ ModelRequest, sink ModelStreamEventSink) (ModelResponse, error) {
	g.mu.Lock()
	g.calls++
	g.mu.Unlock()
	for _, event := range g.events {
		emitModelStreamEvent(sink, event)
	}
	return g.response, g.err
}

func (g *scriptedStreamingGateway) callCount() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.calls
}

func TestModelRouterStreamDoesNotFallbackAfterContent(t *testing.T) {
	endpoints := streamFallbackEndpoints()
	router := newFallbackRouter(endpoints)
	primaryErr := fmt.Errorf("stream disconnected")
	primary := &scriptedStreamingGateway{
		events: []ModelStreamEvent{
			{Type: ModelStreamEventTextDelta, Delta: "partial"},
			{Type: ModelStreamEventError, Err: primaryErr},
		},
		err: primaryErr,
	}
	fallback := &scriptedStreamingGateway{
		response: ModelResponse{Content: "replacement"},
		events: []ModelStreamEvent{
			{Type: ModelStreamEventTextDelta, Delta: "replacement"},
		},
	}
	injectGateway(router, "default/primary", primary, "1")
	injectGateway(router, "default/fallback", fallback, "1")

	var events []ModelStreamEvent
	_, err := router.Stream(context.Background(), ModelRequest{
		Namespace: "default", ModelRef: "primary", FallbackModelRefs: []string{"fallback"},
	}, func(event ModelStreamEvent) {
		events = append(events, event)
	})
	if err == nil || !strings.Contains(err.Error(), "disconnected") {
		t.Fatalf("expected primary stream error, got %v", err)
	}
	if fallback.callCount() != 0 {
		t.Fatalf("fallback replayed after content; calls=%d", fallback.callCount())
	}
	assertModelStreamEventTypes(t, events, ModelStreamEventTextDelta, ModelStreamEventError)
}

func TestModelRouterStreamFallsBackBeforeContent(t *testing.T) {
	endpoints := streamFallbackEndpoints()
	router := newFallbackRouter(endpoints)
	primaryErr := &ModelGatewayError{StatusCode: 503, Provider: "primary", Message: "unavailable"}
	primary := &scriptedStreamingGateway{
		events: []ModelStreamEvent{{Type: ModelStreamEventError, Err: primaryErr}},
		err:    primaryErr,
	}
	fallbackResp := ModelResponse{Content: "replacement"}
	fallback := &scriptedStreamingGateway{
		response: fallbackResp,
		events: []ModelStreamEvent{
			{Type: ModelStreamEventTextDelta, Delta: "replacement"},
			{Type: ModelStreamEventCompletion, Response: &fallbackResp},
		},
	}
	injectGateway(router, "default/primary", primary, "1")
	injectGateway(router, "default/fallback", fallback, "1")

	var events []ModelStreamEvent
	resp, err := router.Stream(context.Background(), ModelRequest{
		Namespace: "default", ModelRef: "primary", FallbackModelRefs: []string{"fallback"},
	}, func(event ModelStreamEvent) {
		events = append(events, event)
	})
	if err != nil {
		t.Fatalf("expected fallback success, got %v", err)
	}
	if resp.Content != "replacement" || fallback.callCount() != 1 {
		t.Fatalf("unexpected fallback response=%+v calls=%d", resp, fallback.callCount())
	}
	assertModelStreamEventTypes(t, events, ModelStreamEventTextDelta, ModelStreamEventCompletion)
}

func TestReActExecutionEngineExecutionScopedModelSinkAdaptsLegacyGateway(t *testing.T) {
	engine := NewReActExecutionEngine(nil, legacyStaticGateway{}, nil, time.Millisecond)
	agent := resources.Agent{
		Metadata: resources.ObjectMeta{Name: "stream-test"},
		Spec: resources.AgentSpec{
			Model:    "test-model",
			ModelRef: "test-ref",
			Limits:   resources.AgentLimits{MaxSteps: 1},
		},
	}
	var events []ModelStreamEvent
	result, err := engine.ExecuteWithEventSink(context.Background(), agent, nil, ExecutionEventSink{
		ModelStream: func(event ModelStreamEvent) {
			events = append(events, event)
		},
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result.Output != "legacy output" {
		t.Fatalf("unexpected output %q", result.Output)
	}
	assertModelStreamEventTypes(t, events, ModelStreamEventTextDelta, ModelStreamEventCompletion)
}

func TestReActExecutionEngineKeepsConcurrentEventSinksIsolated(t *testing.T) {
	engine := NewReActExecutionEngine(nil, agentEchoStreamingGateway{}, nil, time.Millisecond)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, name := range []string{"alpha", "beta"} {
		name := name
		wg.Add(1)
		go func() {
			defer wg.Done()
			agent := resources.Agent{
				Metadata: resources.ObjectMeta{Name: name},
				Spec: resources.AgentSpec{
					Model:    "test-model",
					ModelRef: "test-ref",
					Limits:   resources.AgentLimits{MaxSteps: 1},
				},
			}
			var deltas []string
			_, err := engine.ExecuteWithEventSink(context.Background(), agent, nil, ExecutionEventSink{
				ModelStream: func(event ModelStreamEvent) {
					if event.Type == ModelStreamEventTextDelta {
						deltas = append(deltas, event.Delta)
					}
				},
			})
			if err != nil {
				errs <- fmt.Errorf("%s execution: %w", name, err)
				return
			}
			if len(deltas) != 1 || deltas[0] != name {
				errs <- fmt.Errorf("%s sink received deltas %v", name, deltas)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

type legacyStaticGateway struct{}

func (legacyStaticGateway) Complete(context.Context, ModelRequest) (ModelResponse, error) {
	return ModelResponse{Content: "legacy output"}, nil
}

type agentEchoStreamingGateway struct{}

func (agentEchoStreamingGateway) Complete(context.Context, ModelRequest) (ModelResponse, error) {
	return ModelResponse{}, fmt.Errorf("unexpected blocking completion")
}

func (agentEchoStreamingGateway) Stream(_ context.Context, req ModelRequest, sink ModelStreamEventSink) (ModelResponse, error) {
	resp := ModelResponse{Content: req.Agent}
	emitModelStreamEvent(sink, ModelStreamEvent{Type: ModelStreamEventTextDelta, Delta: req.Agent})
	emitModelStreamEvent(sink, ModelStreamEvent{Type: ModelStreamEventCompletion, Response: &resp})
	return resp, nil
}

func streamFallbackEndpoints() map[string]resources.ModelEndpoint {
	return map[string]resources.ModelEndpoint{
		"default/primary": {
			Metadata: resources.ObjectMeta{Name: "primary", Namespace: "default", ResourceVersion: "1"},
			Spec:     resources.ModelEndpointSpec{Provider: "mock", DefaultModel: "primary"},
		},
		"default/fallback": {
			Metadata: resources.ObjectMeta{Name: "fallback", Namespace: "default", ResourceVersion: "1"},
			Spec:     resources.ModelEndpointSpec{Provider: "mock", DefaultModel: "fallback"},
		},
	}
}

func assertModelStreamEventTypes(t *testing.T, events []ModelStreamEvent, expected ...ModelStreamEventType) {
	t.Helper()
	if len(events) != len(expected) {
		t.Fatalf("expected event types %v, got %v", expected, modelStreamEventTypes(events))
	}
	for i := range expected {
		if events[i].Type != expected[i] {
			t.Fatalf("event %d: expected %q, got %q (all=%v)", i, expected[i], events[i].Type, modelStreamEventTypes(events))
		}
	}
}

func modelStreamEventTypes(events []ModelStreamEvent) []ModelStreamEventType {
	out := make([]ModelStreamEventType, len(events))
	for i := range events {
		out[i] = events[i].Type
	}
	return out
}
