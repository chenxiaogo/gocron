package metrics

import (
	"time"

	"scheduler-platform/pkg/metric"
)

// cacheDurationBuckets for cache operation latency.
var cacheDurationBuckets = []float64{
	0.0001, 0.00025, 0.0005, 0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1,
}

const cacheNamespace = "cache"

// PrometheusMonitor is a Prometheus-based cache monitor.
type PrometheusMonitor struct {
	cacheName string
	opsTotal  metric.CounterVec
	hitsTotal metric.CounterVec
	missesTotal metric.CounterVec
	duration  metric.HistogramVec
	size      metric.GaugeVec
	items     metric.GaugeVec
	hitRate   metric.GaugeVec
	memory    metric.GaugeVec
	evictions metric.CounterVec
}

func NewPrometheusMonitor(mm *metric.MetricManager, cacheName string) *PrometheusMonitor {
	pm := &PrometheusMonitor{cacheName: cacheName}
	pm.registerMetrics(mm)
	return pm
}

func (pm *PrometheusMonitor) registerMetrics(mm *metric.MetricManager) {
	pm.opsTotal = mm.RegisterCounter("cache_operations", &metric.CounterVecOpts{
		VectorOpts: metric.VectorOpts{
			Namespace: cacheNamespace, Subsystem: "core", Name: "operations_total",
			Help: "Total number of cache operations.", Labels: []string{"cache", "operation", "result"},
		},
	})
	pm.hitsTotal = mm.RegisterCounter("cache_hits", &metric.CounterVecOpts{
		VectorOpts: metric.VectorOpts{
			Namespace: cacheNamespace, Subsystem: "core", Name: "hits_total",
			Help: "Total number of cache hits.", Labels: []string{"cache", "level"},
		},
	})
	pm.missesTotal = mm.RegisterCounter("cache_misses", &metric.CounterVecOpts{
		VectorOpts: metric.VectorOpts{
			Namespace: cacheNamespace, Subsystem: "core", Name: "misses_total",
			Help: "Total number of cache misses.", Labels: []string{"cache", "level"},
		},
	})
	pm.duration = mm.RegisterHistogram("cache_operation_duration", &metric.HistogramVecOpts{
		VectorOpts: metric.VectorOpts{
			Namespace: cacheNamespace, Subsystem: "core", Name: "operation_duration_seconds",
			Help: "Cache operation duration in seconds.", Labels: []string{"cache", "operation"},
		},
		Buckets: cacheDurationBuckets,
	})
	pm.size = mm.RegisterGauge("cache_size", &metric.GaugeVecOpts{
		VectorOpts: metric.VectorOpts{
			Namespace: cacheNamespace, Subsystem: "core", Name: "size_bytes",
			Help: "Current cache size in bytes.", Labels: []string{"cache", "level"},
		},
	})
	pm.items = mm.RegisterGauge("cache_items", &metric.GaugeVecOpts{
		VectorOpts: metric.VectorOpts{
			Namespace: cacheNamespace, Subsystem: "core", Name: "items_total",
			Help: "Total number of items in cache.", Labels: []string{"cache", "level"},
		},
	})
	pm.hitRate = mm.RegisterGauge("cache_hit_rate", &metric.GaugeVecOpts{
		VectorOpts: metric.VectorOpts{
			Namespace: cacheNamespace, Subsystem: "core", Name: "hit_rate",
			Help: "Cache hit rate (0-1).", Labels: []string{"cache", "level"},
		},
	})
	pm.memory = mm.RegisterGauge("cache_memory", &metric.GaugeVecOpts{
		VectorOpts: metric.VectorOpts{
			Namespace: cacheNamespace, Subsystem: "core", Name: "memory_usage_bytes",
			Help: "Cache memory usage in bytes.", Labels: []string{"cache", "level"},
		},
	})
	pm.evictions = mm.RegisterCounter("cache_evictions", &metric.CounterVecOpts{
		VectorOpts: metric.VectorOpts{
			Namespace: cacheNamespace, Subsystem: "core", Name: "evictions_total",
			Help: "Total number of cache evictions.", Labels: []string{"cache", "level", "reason"},
		},
	})
}

func (pm *PrometheusMonitor) RecordOperation(operation string, success bool, duration time.Duration) {
	result := "success"
	if !success {
		result = "error"
	}
	pm.opsTotal.Add(1, pm.cacheName, operation, result)
	pm.duration.ObserveFloat(duration.Seconds(), pm.cacheName, operation)
}

func (pm *PrometheusMonitor) RecordHit(level string)   { pm.hitsTotal.Inc(pm.cacheName, level) }
func (pm *PrometheusMonitor) RecordMiss(level string)    { pm.missesTotal.Inc(pm.cacheName, level) }

func (pm *PrometheusMonitor) UpdateCacheMetrics(level string, size, items float64) {
	pm.size.Set(size, pm.cacheName, level)
	pm.items.Set(items, pm.cacheName, level)
}

func (pm *PrometheusMonitor) UpdateHitRate(level string, rate float64) {
	pm.hitRate.Set(rate, pm.cacheName, level)
}

func (pm *PrometheusMonitor) UpdateMemoryUsage(level string, memoryBytes float64) {
	pm.memory.Set(memoryBytes, pm.cacheName, level)
}

func (pm *PrometheusMonitor) RecordEviction(level, reason string) {
	pm.evictions.Inc(pm.cacheName, level, reason)
}

func (pm *PrometheusMonitor) Close() {}