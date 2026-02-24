//go:build aws

package aws

import (
	auditv1 "k8s.io/apiserver/pkg/apis/audit/v1"
)

// EnvelopeParser implements cloud.EnvelopeParser for AWS CloudWatch Logs.
//
// EKS writes Kubernetes audit events directly as CloudWatch log events —
// each message body is a single JSON-encoded audit event (no wrapper
// envelope). Some log forwarding setups may batch events into a JSON array.
type EnvelopeParser struct{}

func (p *EnvelopeParser) Parse(body []byte) ([]auditv1.Event, error) {
	return parseCloudWatchEvent(body)
}
