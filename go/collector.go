package main

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
)

var (
	namespace = "sa"
	// Metrics
	up = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "prom_up"),
		"Was the dependancy up",
		[]string{"dependancy"}, nil,
	)

	metricSaInternal = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "service"),
		"Internal Service Availability 1m for interactive, 5m for batch",
		[]string{"product", "type", "endpoint"}, nil,
	)

	metricSaType = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "service_type"),
		"Interactive or Batch Service Availability aggr 1m or 5m respectively",
		[]string{"product", "type"}, nil,
	)

	metricSaOverall = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "service_overall"),
		"Overall Service Availability aggr",
		[]string{"product"}, nil,
	)
)

// Exporter collects Mon metrics. It implements prometheus.Collector interface.
type Exporter struct {
	promURL, saInteractiveAggr, saBatchAggr string
	mapKeyType                              map[string][]string
	mapKeyEndpoint                          map[string][]string
}

// NewExporter returns an initialized Exporter.
func NewExporter(promURL string, mapKeyType map[string][]string, mapKeyEndpoint map[string][]string, saInteractiveAggr string, saBatchAggr string) *Exporter {
	return &Exporter{
		promURL:           promURL,
		mapKeyType:        mapKeyType,
		mapKeyEndpoint:    mapKeyEndpoint,
		saInteractiveAggr: saInteractiveAggr,
		saBatchAggr:       saBatchAggr,
	}
}

// Describe describes all the metrics ever exported by the Mon exporter. It
// implements prometheus.Collector.
func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	ch <- up
	ch <- metricSaInternal
	ch <- metricSaType
	ch <- metricSaOverall
}

// Collect fetches the stats from configured Mon location and delivers them
// as Prometheus metrics. It implements prometheus.Collector.
func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	startProm := time.Now()
	e.CollectPromMetrics(ch)
	end := time.Now()
	log.Info("Collect finished in ", end.Sub(startProm))
}
