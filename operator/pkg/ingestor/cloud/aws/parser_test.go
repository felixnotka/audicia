package aws

import (
	"encoding/json"
	"testing"
)

func makeAuditEvent(auditID, verb, requestURI string) []byte {
	event := map[string]interface{}{
		"auditID":    auditID,
		"verb":       verb,
		"requestURI": requestURI,
	}
	b, _ := json.Marshal(event)
	return b
}

func makeAuditEventArray(events ...[]byte) []byte {
	result := []byte("[")
	for i, e := range events {
		if i > 0 {
			result = append(result, ',')
		}
		result = append(result, e...)
	}
	result = append(result, ']')
	return result
}

func TestParseCloudWatchEvent(t *testing.T) {
	tests := []struct {
		name       string
		input      []byte
		wantEvents int
		wantErr    bool
	}{
		{
			name:       "single audit event",
			input:      makeAuditEvent("a1", "get", "/api/v1/pods"),
			wantEvents: 1,
		},
		{
			name: "array of audit events",
			input: makeAuditEventArray(
				makeAuditEvent("a1", "get", "/api/v1/pods"),
				makeAuditEvent("a2", "list", "/api/v1/services"),
				makeAuditEvent("a3", "create", "/api/v1/configmaps"),
			),
			wantEvents: 3,
		},
		{
			name:       "empty body",
			input:      []byte{},
			wantEvents: 0,
			wantErr:    false,
		},
		{
			name:       "nil body",
			input:      nil,
			wantEvents: 0,
			wantErr:    false,
		},
		{
			name:       "invalid JSON",
			input:      []byte("not json"),
			wantEvents: 0,
			wantErr:    true,
		},
		{
			name:       "invalid JSON array",
			input:      []byte("[not json]"),
			wantEvents: 0,
			wantErr:    true,
		},
		{
			name:       "empty JSON object",
			input:      []byte("{}"),
			wantEvents: 1,
			wantErr:    false,
		},
		{
			name:       "empty JSON array",
			input:      []byte("[]"),
			wantEvents: 0,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events, err := parseCloudWatchEvent(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseCloudWatchEvent() error = %v, wantErr %v", err, tt.wantErr)
			}
			if len(events) != tt.wantEvents {
				t.Errorf("parseCloudWatchEvent() got %d events, want %d", len(events), tt.wantEvents)
			}
		})
	}
}

func TestParseCloudWatchEventFieldExtraction(t *testing.T) {
	input := makeAuditEvent("test-id-456", "create", "/api/v1/namespaces/default/pods")
	events, err := parseCloudWatchEvent(input)
	if err != nil {
		t.Fatalf("parseCloudWatchEvent() error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}

	if string(events[0].AuditID) != "test-id-456" {
		t.Errorf("AuditID = %q, want %q", events[0].AuditID, "test-id-456")
	}
	if events[0].Verb != "create" {
		t.Errorf("Verb = %q, want %q", events[0].Verb, "create")
	}
	if events[0].RequestURI != "/api/v1/namespaces/default/pods" {
		t.Errorf("RequestURI = %q, want %q", events[0].RequestURI, "/api/v1/namespaces/default/pods")
	}
}

func TestParseCloudWatchEventArrayFieldExtraction(t *testing.T) {
	input := makeAuditEventArray(
		makeAuditEvent("arr-1", "get", "/api/v1/pods"),
		makeAuditEvent("arr-2", "delete", "/api/v1/services"),
	)
	events, err := parseCloudWatchEvent(input)
	if err != nil {
		t.Fatalf("parseCloudWatchEvent() error = %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2", len(events))
	}

	if string(events[0].AuditID) != "arr-1" {
		t.Errorf("events[0].AuditID = %q, want %q", events[0].AuditID, "arr-1")
	}
	if string(events[1].AuditID) != "arr-2" {
		t.Errorf("events[1].AuditID = %q, want %q", events[1].AuditID, "arr-2")
	}
}
