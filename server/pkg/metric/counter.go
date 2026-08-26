package metric

import (
	"fmt"

	prom "github.com/prometheus/client_golang/prometheus"
)

// CounterVecOpts defines configuration options for counter vectors.
type CounterVecOpts struct {
	VectorOpts
}

// CounterVec interface represents a counter vector.
type CounterVec interface {
	Inc(labels ...string)
	Add(v float64, labels ...string)
	Close() bool
}

type promCounterVec struct {
	counter *prom.CounterVec
}

func NewCounterVec(cfg *CounterVecOpts) CounterVec {
	if cfg == nil {
		return nil
	}
	vec := prom.NewCounterVec(prom.CounterOpts{
		Namespace: cfg.Namespace,
		Subsystem: cfg.Subsystem,
		Name:      cfg.Name,
		Help:      cfg.Help,
	}, cfg.Labels)
	return &promCounterVec{counter: vec}
}

func RegisterCounter(cfg *CounterVecOpts) (CounterVec, error) {
	if cfg == nil {
		return nil, fmt.Errorf("nil config")
	}
	vec := prom.NewCounterVec(prom.CounterOpts{
		Namespace: cfg.Namespace,
		Subsystem: cfg.Subsystem,
		Name:      cfg.Name,
		Help:      cfg.Help,
	}, cfg.Labels)
	if err := prom.Register(vec); err != nil {
		return nil, fmt.Errorf("failed to register counter %s: %w", cfg.Name, err)
	}
	return &promCounterVec{counter: vec}, nil
}

func (cv *promCounterVec) Add(v float64, labels ...string) {
	cv.counter.WithLabelValues(labels...).Add(v)
}

func (cv *promCounterVec) Inc(labels ...string) {
	cv.counter.WithLabelValues(labels...).Inc()
}

func (cv *promCounterVec) Close() bool {
	return prom.Unregister(cv.counter)
}