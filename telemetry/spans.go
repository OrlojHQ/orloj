package telemetry

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// StartTaskSpan begins a root span for task execution.
func StartTaskSpan(ctx context.Context, taskName, systemName, namespace string, attempt int) (context.Context, trace.Span) {
	return Tracer().Start(ctx, "task.execute",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(
			attribute.String("orloj.task", taskName),
			attribute.String("orloj.system", systemName),
			attribute.String("orloj.namespace", namespace),
			attribute.Int("orloj.attempt", attempt),
		),
	)
}

// StartAgentSpan begins a child span for a single agent step within a task.
func StartAgentSpan(ctx context.Context, agentName, stepID string, attempt int) (context.Context, trace.Span) {
	return Tracer().Start(ctx, "agent.execute",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(
			attribute.String("orloj.agent", agentName),
			attribute.String("orloj.step_id", stepID),
			attribute.Int("orloj.attempt", attempt),
		),
	)
}

// StartMessageSpan begins a span for processing a single agent message.
func StartMessageSpan(ctx context.Context, taskName, messageID, fromAgent, toAgent, branchID string) (context.Context, trace.Span) {
	return Tracer().Start(ctx, "message.process",
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(
			attribute.String("orloj.task", taskName),
			attribute.String("orloj.message_id", messageID),
			attribute.String("orloj.from_agent", fromAgent),
			attribute.String("orloj.to_agent", toAgent),
			attribute.String("orloj.branch_id", branchID),
		),
	)
}

// StartToolSpan begins a child span for a tool execution.
func StartToolSpan(ctx context.Context, toolName string, attempt int) (context.Context, trace.Span) {
	return Tracer().Start(ctx, "tool.execute",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("orloj.tool", toolName),
			attribute.Int("orloj.tool.attempt", attempt),
		),
	)
}

// StartModelSpan begins a child span for a model gateway call.
func StartModelSpan(ctx context.Context, model, agent string) (context.Context, trace.Span) {
	return Tracer().Start(ctx, "model.call",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("orloj.model", model),
			attribute.String("orloj.agent", agent),
		),
	)
}

// EndSpanOK records success attributes and ends the span.
func EndSpanOK(span trace.Span, attrs ...attribute.KeyValue) {
	span.SetAttributes(attrs...)
	span.End()
}

// EndSpanError records the error and ends the span.
func EndSpanError(span trace.Span, err error, attrs ...attribute.KeyValue) {
	span.SetAttributes(attrs...)
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
	span.End()
}

// SetTokenAttributes adds token usage attributes to a span.
func SetTokenAttributes(span trace.Span, tokensUsed, tokensEstimated, toolCalls int) {
	span.SetAttributes(
		attribute.Int("orloj.tokens.used", tokensUsed),
		attribute.Int("orloj.tokens.estimated", tokensEstimated),
		attribute.Int("orloj.tool_calls", toolCalls),
	)
}
