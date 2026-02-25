package ingestor

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

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

// --- FileIngestor ---

func writeAuditFile(t *testing.T, path string, events []string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close() //nolint:errcheck // test helper
	for _, e := range events {
		_, _ = f.WriteString(e + "\n")
	}
}

func TestFileIngestor_ReadFromBeginningOnInodeChange(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("inode detection only works on Linux")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")

	writeAuditFile(t, path, []string{
		validAuditJSON("a1", "get", "pods", "default"),
	})

	// Start ingestor with a different inode (simulating rotation).
	startPos := Position{
		FileOffset: 9999,  // Would skip all content if inode matched.
		Inode:      12345, // Fake inode that won't match the real file.
	}
	ing := NewFileIngestor(path, startPos, 100)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, err := ing.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Should receive the event because inode mismatch resets offset to 0.
	select {
	case event := <-ch:
		if string(event.AuditID) != "a1" {
			t.Errorf("expected auditID=a1, got %s", event.AuditID)
		}
	case <-time.After(4 * time.Second):
		t.Error("timeout: expected event after inode change reset")
	}

	cancel()
	for range ch {
	}
}

func TestFileIngestor_ResumeFromCheckpoint(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")

	// Write 2 events.
	writeAuditFile(t, path, []string{
		validAuditJSON("a1", "get", "pods", "default"),
		validAuditJSON("a2", "list", "pods", "default"),
	})

	// First read: start from beginning.
	ing := NewFileIngestor(path, Position{}, 100)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, err := ing.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}

	var count int
	deadline := time.After(3 * time.Second)
loop:
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				break loop
			}
			count++
			if count >= 2 {
				break loop
			}
		case <-deadline:
			break loop
		}
	}
	cancel()
	for range ch {
	}

	if count != 2 {
		t.Fatalf("expected 2 events on first read, got %d", count)
	}

	// Get checkpoint.
	pos := ing.Checkpoint()
	if pos.FileOffset == 0 {
		t.Fatal("expected non-zero file offset in checkpoint")
	}

	// Start a new ingestor from the checkpoint — should read 0 new events.
	ing2 := NewFileIngestor(path, pos, 100)
	ctx2, cancel2 := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel2()

	ch2, err := ing2.Start(ctx2)
	if err != nil {
		t.Fatal(err)
	}

	// Should not get any events (already read).
	select {
	case <-ch2:
		// This could be triggered if polling finds new data, which shouldn't happen.
	case <-time.After(2 * time.Second):
		// Expected: no new events.
	}
	cancel2()
	for range ch2 {
	}
}

func TestFileIngestor_PollDetectsRotation(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("inode detection only works on Linux")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")

	// Write initial event.
	writeAuditFile(t, path, []string{
		validAuditJSON("a1", "get", "pods", "default"),
	})

	ing := NewFileIngestor(path, Position{}, 100)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ch, err := ing.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Consume the initial event.
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for initial event")
	}

	// Rotate: remove old file and immediately write a new one.
	// The poll tick (1s) detects inode change → pollForData returns →
	// tail() sleeps 2s → readFile() reopens the new file.
	_ = os.Remove(path)
	writeAuditFile(t, path, []string{
		validAuditJSON("b1", "create", "configmaps", "default"),
	})

	// Should eventually get the event from the new file.
	// Allow generous time for poll(1s) + tail-retry(2s) + readFile.
	select {
	case event := <-ch:
		if string(event.AuditID) != "b1" {
			t.Errorf("expected auditID=b1 from rotated file, got %s", event.AuditID)
		}
	case <-time.After(15 * time.Second):
		t.Error("timeout: expected event from rotated file")
	}

	cancel()
	for range ch {
	}
}
