package azure

import (
	"encoding/json"
	"fmt"

	auditv1 "k8s.io/apiserver/pkg/apis/audit/v1"
)

// diagnosticEnvelope is the top-level Azure Diagnostic Settings JSON structure.
type diagnosticEnvelope struct {
	Records []diagnosticRecord `json:"records"`
}

// diagnosticRecord is a single record within the Diagnostic Settings envelope.
type diagnosticRecord struct {
	Category   string           `json:"category"`
	Properties recordProperties `json:"properties"`
}

// recordProperties holds the embedded audit event.
type recordProperties struct {
	// Log contains the JSON-encoded Kubernetes audit event as a string.
	Log string `json:"log"`
}

// auditCategories are the Diagnostic Settings categories that contain
// Kubernetes audit events.
var auditCategories = map[string]bool{
	"kube-audit":       true,
	"kube-audit-admin": true,
}

// parseEnvelope extracts Kubernetes audit events from an Azure Diagnostic
// Settings envelope. This function is the core parsing logic shared between
// the build-tagged EnvelopeParser and the untagged parser tests.
func parseEnvelope(body []byte) ([]auditv1.Event, error) {
	var envelope diagnosticEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("unmarshaling diagnostic envelope: %w", err)
	}

	if len(envelope.Records) == 0 {
		return nil, nil
	}

	var events []auditv1.Event
	for _, rec := range envelope.Records {
		if !auditCategories[rec.Category] {
			continue
		}

		if rec.Properties.Log == "" {
			continue
		}

		var event auditv1.Event
		if err := json.Unmarshal([]byte(rec.Properties.Log), &event); err != nil {
			// Caller (EnvelopeParser.Parse) handles logging; here we just skip.
			continue
		}
		events = append(events, event)
	}
	return events, nil
}
