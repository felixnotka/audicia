package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// EventsProcessedTotal is the total number of audit events processed.
	EventsProcessedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "audicia",
			Name:      "events_processed_total",
			Help:      "Total audit events processed.",
		},
		[]string{"source", "result"},
	)

	// EventsFilteredTotal is the total number of events dropped by the noise filter.
	EventsFilteredTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "audicia",
			Name:      "events_filtered_total",
			Help:      "Events dropped by the noise filter.",
		},
		[]string{"filter_rule"},
	)

	// RulesGeneratedTotal is the total number of unique rules generated.
	RulesGeneratedTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: "audicia",
			Name:      "rules_generated_total",
			Help:      "Unique rules generated across all reports.",
		},
	)

	// ReportsUpdatedTotal is the total number of AudiciaPolicyReport updates.
	ReportsUpdatedTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: "audicia",
			Name:      "reports_updated_total",
			Help:      "Number of AudiciaPolicyReport status updates.",
		},
	)

	// PipelineLatencySeconds is the end-to-end processing latency per event batch.
	PipelineLatencySeconds = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: "audicia",
			Name:      "pipeline_latency_seconds",
			Help:      "End-to-end processing latency per event batch.",
			Buckets:   prometheus.DefBuckets,
		},
	)

	// CheckpointLagSeconds is the time since last successful checkpoint.
	CheckpointLagSeconds = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "audicia",
			Name:      "checkpoint_lag_seconds",
			Help:      "Time since last successful checkpoint.",
		},
		[]string{"source"},
	)

	// ReportRulesCount is the number of rules in each report.
	ReportRulesCount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "audicia",
			Name:      "report_rules_count",
			Help:      "Number of rules in each report.",
		},
		[]string{"report_name"},
	)

	// ReconcileErrorsTotal is the total number of controller reconciliation errors.
	ReconcileErrorsTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: "audicia",
			Name:      "reconcile_errors_total",
			Help:      "Controller reconciliation errors.",
		},
	)
)

func init() {
	metrics.Registry.MustRegister(
		EventsProcessedTotal,
		EventsFilteredTotal,
		RulesGeneratedTotal,
		ReportsUpdatedTotal,
		PipelineLatencySeconds,
		CheckpointLagSeconds,
		ReportRulesCount,
		ReconcileErrorsTotal,
	)
}
