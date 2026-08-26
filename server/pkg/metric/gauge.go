package metric

import (
	"fmt"

	prom "github.com/prometheus/client_golang/prometheus"
)

// GaugeVecOpts defines configuration options for gauge vectors.
type GaugeVecOpts struct {
	VectorOpts
}

// GaugeVec represents a gauge vector.
type GaugeVec interface {
	Set(v float64, labels ...string)
	Inc(labels ...string)
	Dec(labels ...string)
	Add(v float64, labels ...string)
	Sub(v float64, labels ...string)
	Close() bool
}

type promGaugeVec struct {
	gauge *prom.GaugeVec
}

func NewGaugeVec(cfg *GaugeVecOpts) GaugeVec {
	if cfg == nil {
		return nil
	}
	vec := prom.NewGaugeVec(prom.GaugeOpts{
		Namespace: cfg.Namespace,
		Subsystem: cfg.Subsystem,
		Name:      cfg.Name,
		Help:      cfg.Help,
	}, cfg.Labels)
	return &promGaugeVec{gauge: vec}
}

func RegisterGauge(cfg *GaugeVecOpts) (GaugeVec, error) {
	if cfg == nil {
		return nil, fmt.Errorf("nil config")
	}
	vec := prom.NewGaugeVec(prom.GaugeOpts{
		Namespace: cfg.Namespace,
		Subsystem: cfg.Subsystem,
		Name:      cfg.Name,
		Help:      cfg.Help,
	}, cfg.Labels)
	if err := prom.Register(vec); err != nil {
		return nil, fmt.Errorf("failed to register gauge %s: %w", cfg.Name, err)
	}
	return &promGaugeVec{gauge: vec}, nil
}

func (gv *promGaugeVec) Add(v float64, labels ...string) { gv.gauge.WithLabelValues(labels...).Add(v) }
func (gv *promGaugeVec) Dec(labels ...string)            { gv.gauge.WithLabelValues(labels...).Dec() }
func (gv *promGaugeVec) Inc(labels ...string)            { gv.gauge.WithLabelValues(labels...).Inc() }
func (gv *promGaugeVec) Set(v float64, labels ...string) { gv.gauge.WithLabelValues(labels...).Set(v) }
func (gv *promGaugeVec) Sub(v float64, labels ...string) { gv.gauge.WithLabelValues(labels...).Sub(v) }
func (gv *promGaugeVec) Close() bool                     { return prom.Unregister(gv.gauge) }