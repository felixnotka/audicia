package aws

import (
	"encoding/json"
	"fmt"

	auditv1 "k8s.io/apiserver/pkg/apis/audit/v1"
)

// parseCloudWatchEvent extracts Kubernetes audit events from a CloudWatch
// Logs event message body.
//
// EKS writes each audit event as a separate CloudWatch log event. The message
// body is a single JSON-encoded audit event — no wrapper envelope like Azure
// Diagnostic Settings. Some log forwarding setups may batch multiple events
// into a JSON array, so we try array parsing first and fall back to a single
// event.
func parseCloudWatchEvent(body []byte) ([]auditv1.Event, error) {
	if len(body) == 0 {
		return nil, nil
	}

	// Try JSON array first (some forwarding setups batch events).
	if len(body) > 0 && body[0] == '[' {
		var events []auditv1.Event
		if err := json.Unmarshal(body, &events); err != nil {
			return nil, fmt.Errorf("unmarshaling audit event array: %w", err)
		}
		return events, nil
	}

	// Single audit event (standard EKS format).
	var event auditv1.Event
	if err := json.Unmarshal(body, &event); err != nil {
		return nil, fmt.Errorf("unmarshaling audit event: %w", err)
	}
	return []auditv1.Event{event}, nil
}
