package azure

import (
	"encoding/json"
	"testing"
)

func makeEnvelope(records ...diagnosticRecord) []byte {
	env := diagnosticEnvelope{Records: records}
	b, _ := json.Marshal(env)
	return b
}

func makeAuditRecord(category, auditID, verb string) diagnosticRecord {
	event := map[string]interface{}{
		"auditID":    auditID,
		"verb":       verb,
		"requestURI": "/api/v1/pods",
	}
	eventJSON, _ := json.Marshal(event)
	return diagnosticRecord{
		Category:   category,
		Properties: recordProperties{Log: string(eventJSON)},
	}
}

func TestEnvelopeParsing(t *testing.T) {
	tests := []struct {
		name       string
		input      []byte
		wantEvents int
		wantErr    bool
	}{
		{
			name:       "single kube-audit record",
			input:      makeEnvelope(makeAuditRecord("kube-audit", "a1", "get")),
			wantEvents: 1,
		},
		{
			name:       "kube-audit-admin record",
			input:      makeEnvelope(makeAuditRecord("kube-audit-admin", "a1", "create")),
			wantEvents: 1,
		},
		{
			name: "multiple audit records",
			input: makeEnvelope(
				makeAuditRecord("kube-audit", "a1", "get"),
				makeAuditRecord("kube-audit", "a2", "list"),
				makeAuditRecord("kube-audit-admin", "a3", "delete"),
			),
			wantEvents: 3,
		},
		{
			name: "non-audit category skipped",
			input: makeEnvelope(
				diagnosticRecord{Category: "kube-controller-manager", Properties: recordProperties{Log: "{}"}},
				makeAuditRecord("kube-audit", "a1", "get"),
			),
			wantEvents: 1,
		},
		{
			name:       "empty records",
			input:      makeEnvelope(),
			wantEvents: 0,
		},
		{
			name:       "invalid JSON",
			input:      []byte("not json"),
			wantEvents: 0,
			wantErr:    true,
		},
		{
			name: "empty log field skipped",
			input: makeEnvelope(
				diagnosticRecord{Category: "kube-audit", Properties: recordProperties{Log: ""}},
				makeAuditRecord("kube-audit", "a1", "get"),
			),
			wantEvents: 1,
		},
		{
			name: "malformed audit event in log skipped",
			input: makeEnvelope(
				diagnosticRecord{Category: "kube-audit", Properties: recordProperties{Log: "not valid json"}},
				makeAuditRecord("kube-audit", "a1", "get"),
			),
			wantEvents: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events, err := parseEnvelope(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseEnvelope() error = %v, wantErr %v", err, tt.wantErr)
			}
			if len(events) != tt.wantEvents {
				t.Errorf("parseEnvelope() got %d events, want %d", len(events), tt.wantEvents)
			}
		})
	}
}

func TestEnvelopeFieldExtraction(t *testing.T) {
	input := makeEnvelope(makeAuditRecord("kube-audit", "test-id-123", "create"))
	events, err := parseEnvelope(input)
	if err != nil {
		t.Fatalf("parseEnvelope() error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}

	if string(events[0].AuditID) != "test-id-123" {
		t.Errorf("AuditID = %q, want %q", events[0].AuditID, "test-id-123")
	}
	if events[0].Verb != "create" {
		t.Errorf("Verb = %q, want %q", events[0].Verb, "create")
	}
	if events[0].RequestURI != "/api/v1/pods" {
		t.Errorf("RequestURI = %q, want %q", events[0].RequestURI, "/api/v1/pods")
	}
}
