//go:build azure

package azure

import (
	auditv1 "k8s.io/apiserver/pkg/apis/audit/v1"
)

// EnvelopeParser implements cloud.EnvelopeParser for Azure Diagnostic Settings.
//
// Azure wraps Kubernetes audit events in a Diagnostic Settings envelope:
//
//	{
//	  "records": [
//	    {
//	      "category": "kube-audit" | "kube-audit-admin",
//	      "properties": { "log": "<JSON-encoded audit event>" }
//	    }
//	  ]
//	}
//
// Some messages may contain non-audit records (e.g., activity logs). These
// records are silently skipped.
type EnvelopeParser struct{}

func (p *EnvelopeParser) Parse(body []byte) ([]auditv1.Event, error) {
	return parseEnvelope(body)
}
