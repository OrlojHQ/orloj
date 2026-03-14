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

func TestAzureOpenAIModelGatewayCompleteSuccess(t *testing.T) {
	type capturedRequest struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}

	captured := capturedRequest{}
	var capturedPath string
	var capturedQuery string
	var capturedAPIKey string

	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			capturedPath = req.URL.Path
			capturedQuery = req.URL.RawQuery
			capturedAPIKey = req.Header.Get("api-key")
			body, err := io.ReadAll(req.Body)
			if err != nil {
				return nil, err
			}
			if err := json.Unmarshal(body, &captured); err != nil {
				return nil, err
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"choices":[{"message":{"content":"hello from azure"}}]}`)),
				Header:     make(http.Header),
			}, nil
		}),
		Timeout: time.Second,
	}

	cfg := DefaultAzureOpenAIModelGatewayConfig()
	cfg.APIKey = "azure-key"
	cfg.BaseURL = "https://example.openai.azure.com"
	cfg.DefaultDeployment = "gpt4o-deployment"
	cfg.APIVersion = "2024-10-21"
	cfg.HTTPClient = client

	gateway, err := NewAzureOpenAIModelGateway(cfg)
	if err != nil {
		t.Fatalf("new gateway failed: %v", err)
	}

	resp, err := gateway.Complete(context.Background(), ModelRequest{Prompt: "test", Step: 1})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if resp.Content != "hello from azure" {
		t.Fatalf("unexpected response content %q", resp.Content)
	}
	if capturedAPIKey != "azure-key" {
		t.Fatalf("unexpected api-key header %q", capturedAPIKey)
	}
	if capturedPath != "/openai/deployments/gpt4o-deployment/chat/completions" {
		t.Fatalf("unexpected request path %q", capturedPath)
	}
	if !strings.Contains(capturedQuery, "api-version=2024-10-21") {
		t.Fatalf("unexpected query string %q", capturedQuery)
	}
	if captured.Model != "gpt4o-deployment" {
		t.Fatalf("expected deployment model, got %q", captured.Model)
	}
}

func TestAzureOpenAIModelGatewayCompleteProviderError(t *testing.T) {
	client := &http.Client{
		Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusForbidden,
				Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"forbidden"}}`)),
				Header:     make(http.Header),
			}, nil
		}),
		Timeout: time.Second,
	}

	cfg := DefaultAzureOpenAIModelGatewayConfig()
	cfg.APIKey = "azure-key"
	cfg.BaseURL = "https://example.openai.azure.com"
	cfg.DefaultDeployment = "deployment"
	cfg.HTTPClient = client

	gateway, err := NewAzureOpenAIModelGateway(cfg)
	if err != nil {
		t.Fatalf("new gateway failed: %v", err)
	}
	_, err = gateway.Complete(context.Background(), ModelRequest{Step: 1})
	if err == nil {
		t.Fatal("expected provider error")
	}
	if !strings.Contains(err.Error(), "forbidden") {
		t.Fatalf("expected forbidden in error, got %v", err)
	}
}

func TestAzureOpenAIModelGatewayCompleteRequestFailure(t *testing.T) {
	client := &http.Client{
		Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			return nil, fmt.Errorf("transport unavailable")
		}),
		Timeout: time.Second,
	}

	cfg := DefaultAzureOpenAIModelGatewayConfig()
	cfg.APIKey = "azure-key"
	cfg.BaseURL = "https://example.openai.azure.com"
	cfg.DefaultDeployment = "deployment"
	cfg.HTTPClient = client

	gateway, err := NewAzureOpenAIModelGateway(cfg)
	if err != nil {
		t.Fatalf("new gateway failed: %v", err)
	}
	_, err = gateway.Complete(context.Background(), ModelRequest{Step: 1})
	if err == nil {
		t.Fatal("expected transport error")
	}
	if !strings.Contains(err.Error(), "transport unavailable") {
		t.Fatalf("expected transport unavailable in error, got %v", err)
	}
}

func TestAzureOpenAIModelGatewayCompleteToolCallResponse(t *testing.T) {
	client := &http.Client{
		Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(
					`{"choices":[{"message":{"content":null,"tool_calls":[{"type":"function","function":{"name":"web_search","arguments":"{\"input\":\"status report\"}"}}]}}]}`,
				)),
				Header: make(http.Header),
			}, nil
		}),
		Timeout: time.Second,
	}

	cfg := DefaultAzureOpenAIModelGatewayConfig()
	cfg.APIKey = "azure-key"
	cfg.BaseURL = "https://example.openai.azure.com"
	cfg.DefaultDeployment = "deployment"
	cfg.HTTPClient = client

	gateway, err := NewAzureOpenAIModelGateway(cfg)
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
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected one tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Name != "web_search" {
		t.Fatalf("unexpected tool name %q", resp.ToolCalls[0].Name)
	}
}
