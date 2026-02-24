//go:build gcp

package gcp

import (
	auditv1 "k8s.io/apiserver/pkg/apis/audit/v1"
)

// EnvelopeParser implements cloud.EnvelopeParser for GCP Cloud Logging.
//
// GKE audit events routed to Pub/Sub via a Cloud Logging sink arrive as
// LogEntry JSON objects with the audit data in protoPayload. The parser
// converts these to native Kubernetes audit events.
//
// As a fallback, raw Kubernetes audit events (e.g., from Fluentd/Vector
// pipelines) are auto-detected and passed through unchanged.
type EnvelopeParser struct{}

func (p *EnvelopeParser) Parse(body []byte) ([]auditv1.Event, error) {
	return parseLogEntry(body)
}
