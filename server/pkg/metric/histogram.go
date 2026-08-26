package metric

import (
	"fmt"

	prom "github.com/prometheus/client_golang/prometheus"
)

// HistogramVecOpts defines configuration options for histogram vectors.
type HistogramVecOpts struct {
	VectorOpts
	Buckets []float64
}

// HistogramVec represents a histogram vector.
type HistogramVec interface {
	Observe(v int64, labels ...string)
	ObserveFloat(v float64, labels ...string)
	Close() bool
}

type promHistogramVec struct {
	histogram *prom.HistogramVec
}

func NewHistogramVec(cfg *HistogramVecOpts) HistogramVec {
	if cfg == nil {
		return nil
	}
	vec := prom.NewHistogramVec(prom.HistogramOpts{
		Namespace: cfg.Namespace,
		Subsystem: cfg.Subsystem,
		Name:      cfg.Name,
		Help:      cfg.Help,
		Buckets:   cfg.Buckets,
	}, cfg.Labels)
	return &promHistogramVec{histogram: vec}
}

func RegisterHistogram(cfg *HistogramVecOpts) (HistogramVec, error) {
	if cfg == nil {
		return nil, fmt.Errorf("nil config")
	}
	vec := prom.NewHistogramVec(prom.HistogramOpts{
		Namespace: cfg.Namespace,
		Subsystem: cfg.Subsystem,
		Name:      cfg.Name,
		Help:      cfg.Help,
		Buckets:   cfg.Buckets,
	}, cfg.Labels)
	if err := prom.Register(vec); err != nil {
		return nil, fmt.Errorf("failed to register histogram %s: %w", cfg.Name, err)
	}
	return &promHistogramVec{histogram: vec}, nil
}

func (hv *promHistogramVec) Observe(v int64, labels ...string) {
	hv.histogram.WithLabelValues(labels...).Observe(float64(v))
}

func (hv *promHistogramVec) ObserveFloat(v float64, labels ...string) {
	hv.histogram.WithLabelValues(labels...).Observe(v)
}

func (hv *promHistogramVec) Close() bool {
	return prom.Unregister(hv.histogram)
}