# Prometheus Metrics Reference

Audicia exposes Prometheus metrics on its `/metrics` endpoint (default `:8080`).
These metrics provide visibility into the operator's processing pipeline, report
generation, and health.

## Metrics

All metrics use the `audicia_` namespace.

| Metric                             | Type      | Labels             | Description                                                                                                                                                                                                                 |
| ---------------------------------- | --------- | ------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `audicia_events_processed_total`   | Counter   | `source`, `result` | Total audit events processed (increments after filter + normalizer, before aggregator). `result` is `accepted`, `filtered`, or `error`. A spike in `accepted` events is a reliable signal for new policy-relevant activity. |
| `audicia_events_filtered_total`    | Counter   | `filter_rule`      | Events dropped by the noise filter. `filter_rule` is `deny` (explicit filter match) or `system_user` (ignoreSystemUsers).                                                                                                   |
| `audicia_rules_generated_total`    | Counter   | -                  | Unique rules generated across all reports.                                                                                                                                                                                  |
| `audicia_reports_updated_total`    | Counter   | -                  | Number of AudiciaPolicyReport status updates.                                                                                                                                                                               |
| `audicia_pipeline_latency_seconds` | Histogram | -                  | End-to-end processing latency per flush cycle (seconds).                                                                                                                                                                    |
| `audicia_checkpoint_lag_seconds`   | Gauge     | `source`           | Time since last successful checkpoint. Reset to 0 on each flush. Alerts if consistently high.                                                                                                                               |
| `audicia_report_rules_count`       | Gauge     | `report_name`      | Number of rules in each report. Useful for monitoring report growth.                                                                                                                                                        |
| `audicia_reconcile_errors_total`   | Counter   | -                  | Controller reconciliation errors.                                                                                                                                                                                           |

### Cloud Ingestion Metrics

| Metric                                      | Type      | Labels                  | Description                                                                                               |
| ------------------------------------------- | --------- | ----------------------- | --------------------------------------------------------------------------------------------------------- |
| `audicia_cloud_messages_received_total`     | Counter   | `provider`, `partition` | Total cloud messages received from the message bus, per provider and partition.                           |
| `audicia_cloud_messages_acked_total`        | Counter   | `provider`              | Total cloud message batches acknowledged after successful processing.                                     |
| `audicia_cloud_receive_errors_total`        | Counter   | `provider`              | Total errors receiving from the cloud message bus. Sustained non-zero rate indicates connectivity issues. |
| `audicia_cloud_lag_seconds`                 | Histogram | `provider`              | Lag between message enqueue time and processing time. High values mean the consumer is falling behind.    |
| `audicia_cloud_envelope_parse_errors_total` | Counter   | `provider`              | Total errors parsing cloud provider envelopes. Non-zero values may indicate envelope format changes.      |

## Scrape Configuration

### ServiceMonitor (Prometheus Operator)

Enable the ServiceMonitor in Helm values:

```yaml
serviceMonitor:
  enabled: true
  labels:
    release: prometheus # Match your Prometheus selector
  interval: 30s
```

### Manual Scrape Config

If not using the Prometheus Operator:

```yaml
scrape_configs:
  - job_name: audicia-operator
    kubernetes_sd_configs:
      - role: service
        namespaces:
          names: [audicia-system]
    relabel_configs:
      - source_labels: [__meta_kubernetes_service_name]
        regex: audicia-.*-metrics
        action: keep
```

## Health Probes

| Probe     | Endpoint   | Port | Description                                  |
| --------- | ---------- | ---- | -------------------------------------------- |
| Liveness  | `/healthz` | 8081 | Basic ping check — operator process is alive |
| Readiness | `/readyz`  | 8081 | Operator is ready to process events          |

## Example Alerts

### Pipeline Stalled

Alert when a source hasn't checkpointed in over 5 minutes:

```yaml
- alert: AudiciaCheckpointStalled
  expr: audicia_checkpoint_lag_seconds > 300
  for: 5m
  labels:
    severity: warning
  annotations:
    summary: "Audicia pipeline stalled for source {{ $labels.source }}"
    description: "No checkpoint in {{ $value }}s. Check operator logs."
```

### High Error Rate

Alert when more than 10% of events are errors:

```yaml
- alert: AudiciaHighErrorRate
  expr: |
    rate(audicia_events_processed_total{result="error"}[5m])
    / rate(audicia_events_processed_total[5m]) > 0.1
  for: 10m
  labels:
    severity: warning
  annotations:
    summary: "Audicia error rate above 10%"
```

### Cloud Consumer Lag

Alert when cloud ingestion lag exceeds 2 minutes:

```yaml
- alert: AudiciaCloudLagHigh
  expr: histogram_quantile(0.95, rate(audicia_cloud_lag_seconds_bucket[5m])) > 120
  for: 10m
  labels:
    severity: warning
  annotations:
    summary: "Cloud ingestion lag above 2 minutes for provider {{ $labels.provider }}"
    description: "p95 lag is {{ $value }}s. Check Event Hub consumer group lag."
```

### Report Growth

Alert when a report exceeds 150 rules (approaching the default 200 limit):

```yaml
- alert: AudiciaReportNearLimit
  expr: audicia_report_rules_count > 150
  for: 5m
  labels:
    severity: info
  annotations:
    summary: "Report {{ $labels.report_name }} has {{ $value }} rules (limit: 200)"
```
