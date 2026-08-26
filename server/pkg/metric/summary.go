package metric

import (
	"fmt"

	prom "github.com/prometheus/client_golang/prometheus"
)

// SummaryVecOpts defines configuration options for summary vectors.
type SummaryVecOpts struct {
	VectorOpts
	Objectives map[float64]float64
}

// SummaryVec represents a summary vector.
type SummaryVec interface {
	Observe(v float64, labels ...string)
	Close() bool
}

type promSummaryVec struct {
	summary *prom.SummaryVec
}

func NewSummaryVec(cfg *SummaryVecOpts) SummaryVec {
	if cfg == nil {
		return nil
	}
	vec := prom.NewSummaryVec(prom.SummaryOpts{
		Namespace:  cfg.Namespace,
		Subsystem:  cfg.Subsystem,
		Name:       cfg.Name,
		Help:       cfg.Help,
		Objectives: cfg.Objectives,
	}, cfg.Labels)
	return &promSummaryVec{summary: vec}
}

func RegisterSummary(cfg *SummaryVecOpts) (SummaryVec, error) {
	if cfg == nil {
		return nil, fmt.Errorf("nil config")
	}
	vec := prom.NewSummaryVec(prom.SummaryOpts{
		Namespace:  cfg.Namespace,
		Subsystem:  cfg.Subsystem,
		Name:       cfg.Name,
		Help:       cfg.Help,
		Objectives: cfg.Objectives,
	}, cfg.Labels)
	if err := prom.Register(vec); err != nil {
		return nil, fmt.Errorf("failed to register summary %s: %w", cfg.Name, err)
	}
	return &promSummaryVec{summary: vec}, nil
}

func (sv *promSummaryVec) Observe(v float64, labels ...string) {
	sv.summary.WithLabelValues(labels...).Observe(v)
}

func (sv *promSummaryVec) Close() bool {
	return prom.Unregister(sv.summary)
}