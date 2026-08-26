// Package trace 提供轻量自研的链路 ID（trace id / span id）注入与提取。
//
// 替代 go-zero 内部依赖 OpenTelemetry 的实现：直接用 context 键存储字符串形式的
// trace id 与 span id，彻底移除对 go.opentelemetry.io/otel 的依赖，实现完全自研隔离。
package trace

import "context"

type contextKey int

const (
	traceIDKey contextKey = iota
	spanIDKey
)

// ContextWithTraceID 返回一个绑定了 trace id 的 context。
func ContextWithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceIDKey, traceID)
}

// ContextWithSpanID 返回一个绑定了 span id 的 context。
func ContextWithSpanID(ctx context.Context, spanID string) context.Context {
	return context.WithValue(ctx, spanIDKey, spanID)
}

// TraceIDFromContext 从 context 中提取 trace id，未注入则返回空串。
func TraceIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(traceIDKey).(string); ok {
		return v
	}

	return ""
}

// SpanIDFromContext 从 context 中提取 span id，未注入则返回空串。
func SpanIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(spanIDKey).(string); ok {
		return v
	}

	return ""
}
