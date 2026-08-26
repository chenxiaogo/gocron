package metric

import (
	"sync"

	prom "github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

// MetricManager provides unified management for all metrics.
type MetricManager struct {
	mu         sync.Mutex
	logger     *zap.Logger
	collectors []prom.Collector
	counters   map[string]CounterVec
	gauges     map[string]GaugeVec
	histograms map[string]HistogramVec
	summaries  map[string]SummaryVec
}

var (
	managerInstance *MetricManager
	once            sync.Once
)

// GetMetricManager returns the global metric manager instance (singleton).
func GetMetricManager() *MetricManager {
	once.Do(func() {
		managerInstance = &MetricManager{
			logger:     zap.NewNop(),
			counters:   make(map[string]CounterVec),
			gauges:     make(map[string]GaugeVec),
			histograms: make(map[string]HistogramVec),
			summaries:  make(map[string]SummaryVec),
		}
	})
	return managerInstance
}

// SetLogger sets the logger for the metric manager.
func (m *MetricManager) SetLogger(logger *zap.Logger) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.logger = logger
}

// RegisterCounter registers a counter metric.
func (m *MetricManager) RegisterCounter(name string, opts *CounterVecOpts) CounterVec {
	m.mu.Lock()
	defer m.mu.Unlock()

	if cv, exists := m.counters[name]; exists {
		return cv
	}
	cv, err := RegisterCounter(opts)
	if err != nil {
		m.logger.Error("failed to register counter metric", zap.String("name", name), zap.Error(err))
		return nil
	}
	m.counters[name] = cv
	if pcv, ok := cv.(*promCounterVec); ok {
		m.collectors = append(m.collectors, pcv.counter)
	}
	return cv
}

// RegisterGauge registers a gauge metric.
func (m *MetricManager) RegisterGauge(name string, opts *GaugeVecOpts) GaugeVec {
	m.mu.Lock()
	defer m.mu.Unlock()

	if gv, exists := m.gauges[name]; exists {
		return gv
	}
	gv, err := RegisterGauge(opts)
	if err != nil {
		m.logger.Error("failed to register gauge metric", zap.String("name", name), zap.Error(err))
		return nil
	}
	m.gauges[name] = gv
	if pgv, ok := gv.(*promGaugeVec); ok {
		m.collectors = append(m.collectors, pgv.gauge)
	}
	return gv
}

// RegisterHistogram registers a histogram metric.
func (m *MetricManager) RegisterHistogram(name string, opts *HistogramVecOpts) HistogramVec {
	m.mu.Lock()
	defer m.mu.Unlock()

	if hv, exists := m.histograms[name]; exists {
		return hv
	}
	hv, err := RegisterHistogram(opts)
	if err != nil {
		m.logger.Error("failed to register histogram metric", zap.String("name", name), zap.Error(err))
		return nil
	}
	m.histograms[name] = hv
	if phv, ok := hv.(*promHistogramVec); ok {
		m.collectors = append(m.collectors, phv.histogram)
	}
	return hv
}

// RegisterSummary registers a summary metric.
func (m *MetricManager) RegisterSummary(name string, opts *SummaryVecOpts) SummaryVec {
	m.mu.Lock()
	defer m.mu.Unlock()

	if sv, exists := m.summaries[name]; exists {
		return sv
	}
	sv, err := RegisterSummary(opts)
	if err != nil {
		m.logger.Error("failed to register summary metric", zap.String("name", name), zap.Error(err))
		return nil
	}
	m.summaries[name] = sv
	if psv, ok := sv.(*promSummaryVec); ok {
		m.collectors = append(m.collectors, psv.summary)
	}
	return sv
}

// InitializeDefaultMetrics 注册默认 HTTP 指标。
func (m *MetricManager) InitializeDefaultMetrics() {
	m.RegisterCounter("http_requests", &CounterVecOpts{
		VectorOpts: VectorOpts{Namespace: "http", Subsystem: "server", Name: "requests_total",
			Help: "Total HTTP requests", Labels: []string{"method", "path", "status"}},
	})
	m.RegisterHistogram("http_request_duration", &HistogramVecOpts{
		VectorOpts: VectorOpts{Namespace: "http", Subsystem: "server", Name: "request_duration_seconds",
			Help: "HTTP request duration", Labels: []string{"method", "path"}},
		Buckets: []float64{0.05, 0.1, 0.5, 1, 2.5, 5},
	})
	m.RegisterGauge("http_active_requests", &GaugeVecOpts{
		VectorOpts: VectorOpts{Namespace: "http", Subsystem: "server", Name: "active_requests",
			Help: "Active HTTP requests", Labels: []string{"method", "path"}},
	})
}

// CloseAll unregisters all metrics from the Prometheus registry.
func (m *MetricManager) CloseAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, c := range m.collectors {
		prom.Unregister(c)
	}
	m.counters = make(map[string]CounterVec)
	m.gauges = make(map[string]GaugeVec)
	m.histograms = make(map[string]HistogramVec)
	m.summaries = make(map[string]SummaryVec)
	m.collectors = nil
}

// GetCounter returns a registered counter by name.
func (m *MetricManager) GetCounter(name string) CounterVec {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.counters[name]
}

// GetGauge returns a registered gauge by name.
func (m *MetricManager) GetGauge(name string) GaugeVec {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.gauges[name]
}

// GetHistogram returns a registered histogram by name.
func (m *MetricManager) GetHistogram(name string) HistogramVec {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.histograms[name]
}

// GetSummary returns a registered summary by name.
func (m *MetricManager) GetSummary(name string) SummaryVec {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.summaries[name]
}