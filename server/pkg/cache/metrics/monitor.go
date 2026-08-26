package metrics

import (
	"time"

	"scheduler-platform/pkg/metric"
)

// Monitor is the cache monitoring interface.
type Monitor interface {
	RecordOperation(operation string, success bool, duration time.Duration)
	RecordHit(level string)
	RecordMiss(level string)
	UpdateCacheMetrics(level string, size, items float64)
	UpdateHitRate(level string, rate float64)
	UpdateMemoryUsage(level string, memoryBytes float64)
	RecordEviction(level, reason string)
	Close()
}

// NoopMonitor is a no-op monitor.
type NoopMonitor struct{}

func NewNoopMonitor() *NoopMonitor                                                       { return &NoopMonitor{} }
func (m *NoopMonitor) RecordOperation(operation string, success bool, duration time.Duration) {}
func (m *NoopMonitor) RecordHit(level string)                                                 {}
func (m *NoopMonitor) RecordMiss(level string)                                                {}
func (m *NoopMonitor) UpdateCacheMetrics(level string, size, items float64)                   {}
func (m *NoopMonitor) UpdateHitRate(level string, rate float64)                                {}
func (m *NoopMonitor) UpdateMemoryUsage(level string, memoryBytes float64)                     {}
func (m *NoopMonitor) RecordEviction(level, reason string)                                     {}
func (m *NoopMonitor) Close()                                                                  {}

// MonitorFactory creates monitors.
type MonitorFactory struct {
	MetricManager *metric.MetricManager
}

func (f *MonitorFactory) GetMonitor(monitorType, cacheName string) Monitor {
	switch monitorType {
	case "prometheus":
		if f.MetricManager == nil {
			return NewNoopMonitor()
		}
		return NewPrometheusMonitor(f.MetricManager, cacheName)
	case "noop":
		return NewNoopMonitor()
	default:
		return NewNoopMonitor()
	}
}