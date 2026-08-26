// Package tracex 提供基于 OpenTelemetry 的链路 ID（trace id）注入中间件。
//
// 使用 go.opentelemetry.io/otel/trace 的 SpanContext 表示链路 ID：
// HTTP 请求进入时从请求头提取（X-Trace-Id / traceparent）或生成链路 ID，
// 构造 OTel trace.SpanContext 并注入 c.Request 的 context，从而让
// ilog.Trace(ctx) / ilog.TraceZapFields(ctx) 打印出的日志带上 trace_id / span_id。
// 仅依赖 OTel trace API（无 SDK），实现轻量。
package tracex

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/trace"
)

// TraceHeader 是链路 ID 请求头。
const TraceHeader = "X-Trace-Id"

// TraceMiddleware 注入链路 ID 到请求 context 与响应头。
func TraceMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		r := c.Request
		sc := resolveSpanContext(r.Header)

		// 用 OTel SpanContext 注入请求 context
		ctx := trace.ContextWithSpanContext(r.Context(), sc)

		// 回写响应头，方便前端/排查关联
		c.Header(TraceHeader, sc.TraceID().String())

		c.Request = r.WithContext(ctx)
		c.Next()
	}
}

// FromRequest 从 Gin 请求上下文取出带链路 ID 的 context。
// 该 context 由 TraceMiddleware 注入 trace.SpanContext，可直接传给
// ilog.Trace(ctx) / ilog.TraceZapFields(ctx) 或下层方法。
func FromRequest(c *gin.Context) context.Context {
	return c.Request.Context()
}

// TraceIDShort 从 context 提取 trace ID 短串（前 8 位），用于日志消息。
func TraceIDShort(ctx context.Context) string {
	tid := trace.SpanFromContext(ctx).SpanContext().TraceID().String()
	if len(tid) > 8 {
		return tid[:8]
	}
	return tid
}

// NewContext 将新生成的链路 ID 注入任意 context（供非 HTTP 的后台任务使用）。
func NewContext(ctx context.Context) context.Context {
	sc := newSpanContext(newTraceID(), newSpanID())
	return trace.ContextWithSpanContext(ctx, sc)
}

// resolveSpanContext 从请求头提取或生成 OTel SpanContext。
func resolveSpanContext(h http.Header) trace.SpanContext {
	// 优先 go-zero 约定的 X-Trace-Id（32 位 hex）
	if v := strings.TrimSpace(h.Get(TraceHeader)); v != "" {
		if tid := parseTraceID(v); tid.IsValid() {
			return newSpanContext(tid, newSpanID())
		}
	}
	// 其次 W3C traceparent：version-traceid-spanid-flags
	if tp := strings.TrimSpace(h.Get("Traceparent")); tp != "" {
		parts := strings.Split(tp, "-")
		if len(parts) == 4 {
			tid := parseTraceID(parts[1])
			sid := parseSpanID(parts[2])
			if tid.IsValid() {
				if !sid.IsValid() {
					sid = newSpanID()
				}
				return newSpanContext(tid, sid)
			}
		}
	}
	return newSpanContext(newTraceID(), newSpanID())
}

// newSpanContext 构造 OTel SpanContext（通过 NewSpanContext 以兼容未导出字段）。
func newSpanContext(tid trace.TraceID, sid trace.SpanID) trace.SpanContext {
	return trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    tid,
		SpanID:     sid,
		TraceFlags: trace.FlagsSampled,
	})
}

// parseTraceID 解析 32 位 hex 的 trace id。
func parseTraceID(v string) trace.TraceID {
	v = strings.ToLower(strings.TrimSpace(v))
	if len(v) != 32 || !isHex(v) {
		return trace.TraceID{}
	}
	var tid trace.TraceID
	hex.Decode(tid[:], []byte(v))
	return tid
}

// parseSpanID 解析 16 位 hex 的 span id。
func parseSpanID(v string) trace.SpanID {
	v = strings.ToLower(strings.TrimSpace(v))
	if len(v) != 16 || !isHex(v) {
		return trace.SpanID{}
	}
	var sid trace.SpanID
	hex.Decode(sid[:], []byte(v))
	return sid
}

func isHex(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func newTraceID() trace.TraceID {
	var tid trace.TraceID
	if _, err := rand.Read(tid[:]); err != nil {
		// 极低概率随机失败，退化为确定性非零初值
		tid[15] = 1
	}
	if !tid.IsValid() {
		tid[15] = 1
	}
	return tid
}

func newSpanID() trace.SpanID {
	var sid trace.SpanID
	if _, err := rand.Read(sid[:]); err != nil {
		sid[7] = 1
	}
	if !sid.IsValid() {
		sid[7] = 1
	}
	return sid
}
