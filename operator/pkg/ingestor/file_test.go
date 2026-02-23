package ingestor

import (
	"context"
	"strings"
	"testing"

	auditv1 "k8s.io/apiserver/pkg/apis/audit/v1"
)

func TestNewAuditScanner(t *testing.T) {
	r := strings.NewReader("test line\n")
	s := newAuditScanner(r)
	if s == nil {
		t.Fatal("expected non-nil scanner")
	}
	if !s.Scan() {
		t.Error("expected successful scan")
	}
	if s.Text() != "test line" {
		t.Errorf("got %q, want %q", s.Text(), "test line")
	}
}

// validAuditJSON returns a minimal valid audit.k8s.io/v1 Event JSON string.
// Note: requestReceivedTimestamp and stageTimestamp must use RFC3339Micro format
// (exactly 6 decimal places) because metav1.MicroTime.UnmarshalJSON requires it.
func validAuditJSON(auditID, verb, resource, ns string) string {
	return `{"kind":"Event","apiVersion":"audit.k8s.io/v1","metadata":{"creationTimestamp":null},` +
		`"level":"Metadata","auditID":"` + auditID + `","stage":"ResponseComplete",` +
		`"requestURI":"/api/v1/` + resource + `","verb":"` + verb + `",` +
		`"user":{"username":"alice"},` +
		`"objectRef":{"resource":"` + resource + `","namespace":"` + ns + `","apiVersion":"v1"},` +
		`"sourceIPs":["127.0.0.1"],"responseStatus":{"metadata":{},"code":200},` +
		`"requestReceivedTimestamp":"2025-01-01T00:00:00.000000Z","stageTimestamp":"2025-01-01T00:00:01.000000Z"}`
}

func TestScanAndEmit_ValidEvents(t *testing.T) {
	input := validAuditJSON("aaa", "get", "pods", "default") + "\n" +
		validAuditJSON("bbb", "list", "services", "kube-system") + "\n"

	scanner := newAuditScanner(strings.NewReader(input))
	ch := make(chan auditv1.Event, 10)

	readAny, err := scanAndEmit(context.Background(), scanner, ch)
	if err != nil {
		t.Fatal(err)
	}
	if !readAny {
		t.Error("expected readAny = true")
	}
	close(ch)

	var count int
	for e := range ch {
		count++
		if e.Verb == "" {
			t.Error("expected non-empty verb in parsed event")
		}
	}
	if count != 2 {
		t.Errorf("got %d events, want 2", count)
	}
}

func TestScanAndEmit_MalformedLinesSkipped(t *testing.T) {
	input := "not json at all\n" +
		validAuditJSON("ccc", "get", "pods", "default") + "\n" +
		"{broken json\n"

	scanner := newAuditScanner(strings.NewReader(input))
	ch := make(chan auditv1.Event, 10)

	readAny, err := scanAndEmit(context.Background(), scanner, ch)
	if err != nil {
		t.Fatal(err)
	}
	if !readAny {
		t.Error("expected readAny = true (1 valid event)")
	}
	close(ch)

	var count int
	for range ch {
		count++
	}
	if count != 1 {
		t.Errorf("got %d events, want 1 (malformed lines skipped)", count)
	}
}

func TestScanAndEmit_EmptyInput(t *testing.T) {
	scanner := newAuditScanner(strings.NewReader(""))
	ch := make(chan auditv1.Event, 10)

	readAny, err := scanAndEmit(context.Background(), scanner, ch)
	if err != nil {
		t.Fatal(err)
	}
	if readAny {
		t.Error("expected readAny = false for empty input")
	}
}

func TestScanAndEmit_EmptyLinesIgnored(t *testing.T) {
	input := "\n\n" + validAuditJSON("ddd", "get", "pods", "default") + "\n\n"
	scanner := newAuditScanner(strings.NewReader(input))
	ch := make(chan auditv1.Event, 10)

	readAny, err := scanAndEmit(context.Background(), scanner, ch)
	if err != nil {
		t.Fatal(err)
	}
	if !readAny {
		t.Error("expected readAny = true")
	}
	close(ch)

	var count int
	for range ch {
		count++
	}
	if count != 1 {
		t.Errorf("got %d events, want 1 (empty lines ignored)", count)
	}
}

func TestScanAndEmit_ContextCancelled(t *testing.T) {
	// Build a large input.
	var sb strings.Builder
	for i := 0; i < 100; i++ {
		sb.WriteString(validAuditJSON("x", "get", "pods", "default"))
		sb.WriteByte('\n')
	}

	scanner := newAuditScanner(strings.NewReader(sb.String()))
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	ch := make(chan auditv1.Event, 1)
	_, err := scanAndEmit(ctx, scanner, ch)
	if err != nil && err != context.Canceled {
		t.Errorf("unexpected error: %v", err)
	}
}
