package agentruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestOllamaModelGatewayCompleteSuccess(t *testing.T) {
	type capturedRequest struct {
		Model    string `json:"model"`
		Stream   bool   `json:"stream"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}

	captured := capturedRequest{}
	var capturedPath string

	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			capturedPath = req.URL.Path
			body, err := io.ReadAll(req.Body)
			if err != nil {
				return nil, err
			}
			if err := json.Unmarshal(body, &captured); err != nil {
				return nil, err
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"message":{"role":"assistant","content":"hello from ollama"},"done":true}`)),
				Header:     make(http.Header),
			}, nil
		}),
		Timeout: time.Second,
	}

	cfg := DefaultOllamaModelGatewayConfig()
	cfg.BaseURL = "http://localhost:11434"
	cfg.DefaultModel = "llama3.2"
	cfg.HTTPClient = client

	gateway, err := NewOllamaModelGateway(cfg)
	if err != nil {
		t.Fatalf("new gateway failed: %v", err)
	}

	resp, err := gateway.Complete(context.Background(), ModelRequest{Prompt: "test", Step: 2})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if resp.Content != "hello from ollama" {
		t.Fatalf("unexpected response content %q", resp.Content)
	}
	if !resp.Done {
		t.Fatal("expected done=true")
	}
	if capturedPath != "/api/chat" {
		t.Fatalf("unexpected request path %q", capturedPath)
	}
	if captured.Model != "llama3.2" {
		t.Fatalf("expected default model llama3.2, got %q", captured.Model)
	}
	if captured.Stream {
		t.Fatal("expected non-streaming chat request")
	}
}

func TestOllamaModelGatewayCompleteProviderError(t *testing.T) {
	client := &http.Client{
		Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Body:       io.NopCloser(strings.NewReader(`{"error":"unknown model"}`)),
				Header:     make(http.Header),
			}, nil
		}),
		Timeout: time.Second,
	}

	cfg := DefaultOllamaModelGatewayConfig()
	cfg.HTTPClient = client
	gateway, err := NewOllamaModelGateway(cfg)
	if err != nil {
		t.Fatalf("new gateway failed: %v", err)
	}

	_, err = gateway.Complete(context.Background(), ModelRequest{Model: "nonexistent-model", Step: 1})
	if err == nil {
		t.Fatal("expected provider error")
	}
	if !strings.Contains(err.Error(), "unknown model") {
		t.Fatalf("expected unknown model in error, got %v", err)
	}
}

func TestOllamaModelGatewayCompleteRequestFailure(t *testing.T) {
	client := &http.Client{
		Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			return nil, fmt.Errorf("transport unavailable")
		}),
		Timeout: time.Second,
	}

	cfg := DefaultOllamaModelGatewayConfig()
	cfg.HTTPClient = client
	gateway, err := NewOllamaModelGateway(cfg)
	if err != nil {
		t.Fatalf("new gateway failed: %v", err)
	}

	_, err = gateway.Complete(context.Background(), ModelRequest{Model: "llama3.2", Step: 1})
	if err == nil {
		t.Fatal("expected transport error")
	}
	if !strings.Contains(err.Error(), "transport unavailable") {
		t.Fatalf("expected transport unavailable in error, got %v", err)
	}
}

func TestOllamaModelGatewayCompleteToolCallResponse(t *testing.T) {
	type capturedRequest struct {
		Tools []struct {
			Type     string `json:"type"`
			Function struct {
				Name string `json:"name"`
			} `json:"function"`
		} `json:"tools"`
	}

	captured := capturedRequest{}
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			body, err := io.ReadAll(req.Body)
			if err != nil {
				return nil, err
			}
			if err := json.Unmarshal(body, &captured); err != nil {
				return nil, err
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(
					`{"message":{"role":"assistant","content":"","tool_calls":[{"function":{"name":"web_search","arguments":{"input":"market recap"}}}]},"done":false}`,
				)),
				Header: make(http.Header),
			}, nil
		}),
		Timeout: time.Second,
	}

	cfg := DefaultOllamaModelGatewayConfig()
	cfg.BaseURL = "http://localhost:11434"
	cfg.DefaultModel = "llama3.2"
	cfg.HTTPClient = client

	gateway, err := NewOllamaModelGateway(cfg)
	if err != nil {
		t.Fatalf("new gateway failed: %v", err)
	}

	resp, err := gateway.Complete(context.Background(), ModelRequest{
		Step:  1,
		Tools: []string{"web_search"},
	})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if len(captured.Tools) != 1 || captured.Tools[0].Function.Name != "web_search" {
		t.Fatalf("expected request tools payload, got %+v", captured.Tools)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected one tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Name != "web_search" {
		t.Fatalf("unexpected tool call name %q", resp.ToolCalls[0].Name)
	}
	if resp.ToolCalls[0].Input != "market recap" {
		t.Fatalf("unexpected tool call input %q", resp.ToolCalls[0].Input)
	}
}
