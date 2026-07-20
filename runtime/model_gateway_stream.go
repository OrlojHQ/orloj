package agentruntime

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const maxSSEEventBytes = 32 * 1024 * 1024

// streamingHTTPClient preserves the caller's transport and redirect policy but
// removes http.Client's whole-request timeout. Streaming lifetime is governed
// by the request context; a total timeout would abort healthy long responses.
func streamingHTTPClient(client *http.Client) *http.Client {
	if client == nil || client.Timeout == 0 {
		return client
	}
	out := *client
	out.Timeout = 0
	return &out
}

type serverSentEvent struct {
	Type string
	Data string
}

// streamModelResponse uses native streaming when available and otherwise
// adapts a blocking ModelGateway response into the same typed event sequence.
func streamModelResponse(ctx context.Context, gateway ModelGateway, req ModelRequest, sink ModelStreamEventSink) (ModelResponse, error) {
	if streaming, ok := gateway.(StreamingModelGateway); ok {
		return streaming.Stream(ctx, req, sink)
	}
	resp, err := gateway.Complete(ctx, req)
	if err != nil {
		emitModelStreamEvent(sink, ModelStreamEvent{Type: ModelStreamEventError, Err: err})
		return ModelResponse{}, err
	}
	if resp.Content != "" {
		emitModelStreamEvent(sink, ModelStreamEvent{Type: ModelStreamEventTextDelta, Delta: resp.Content})
	}
	for i := range resp.ToolCalls {
		call := resp.ToolCalls[i]
		emitModelStreamEvent(sink, ModelStreamEvent{Type: ModelStreamEventToolCall, ToolCall: &call})
	}
	if modelUsagePresent(resp.Usage) {
		usage := resp.Usage
		emitModelStreamEvent(sink, ModelStreamEvent{Type: ModelStreamEventUsage, Usage: &usage})
	}
	completed := resp
	emitModelStreamEvent(sink, ModelStreamEvent{Type: ModelStreamEventCompletion, Response: &completed})
	return resp, nil
}

func emitModelStreamEvent(sink ModelStreamEventSink, event ModelStreamEvent) {
	if sink != nil {
		sink(event)
	}
}

func emitModelStreamError(sink ModelStreamEventSink, err error) error {
	if err != nil {
		emitModelStreamEvent(sink, ModelStreamEvent{Type: ModelStreamEventError, Err: err})
	}
	return err
}

func modelUsagePresent(usage ModelUsage) bool {
	return usage.InputTokens != 0 || usage.OutputTokens != 0 || usage.TotalTokens != 0 || strings.TrimSpace(usage.Source) != ""
}

// readServerSentEvents parses an SSE stream according to the event-stream
// framing rules, including CRLF, comments, multiline data, and a final event
// that is not followed by a blank line.
func readServerSentEvents(r io.Reader, handle func(serverSentEvent) error) error {
	if r == nil {
		return fmt.Errorf("SSE response body is nil")
	}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), maxSSEEventBytes)

	var eventType string
	dataLines := make([]string, 0, 1)
	dataBytes := 0
	dispatch := func() error {
		if len(dataLines) == 0 {
			eventType = ""
			return nil
		}
		event := serverSentEvent{
			Type: eventType,
			Data: strings.Join(dataLines, "\n"),
		}
		eventType = ""
		dataLines = dataLines[:0]
		dataBytes = 0
		return handle(event)
	}

	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if line == "" {
			if err := dispatch(); err != nil {
				return err
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}

		field, value, found := strings.Cut(line, ":")
		if !found {
			value = ""
		} else if strings.HasPrefix(value, " ") {
			value = value[1:]
		}
		switch field {
		case "event":
			eventType = value
		case "data":
			dataBytes += len(value)
			if dataBytes > maxSSEEventBytes {
				return fmt.Errorf("SSE event exceeds %d bytes", maxSSEEventBytes)
			}
			dataLines = append(dataLines, value)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read SSE stream: %w", err)
	}
	return dispatch()
}
