package cloud

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/types"
	auditv1 "k8s.io/apiserver/pkg/apis/audit/v1"
)

// fakeParser implements EnvelopeParser for testing. It unmarshals the message
// body as a JSON array of audit events.
type fakeParser struct{}

func (p *fakeParser) Parse(body []byte) ([]auditv1.Event, error) {
	var events []auditv1.Event
	if err := json.Unmarshal(body, &events); err != nil {
		return nil, err
	}
	return events, nil
}

func makeMessage(partition, seq, enqueueTime string, events ...auditv1.Event) Message {
	body, _ := json.Marshal(events)
	return Message{
		Body:           body,
		SequenceNumber: seq,
		Partition:      partition,
		EnqueuedTime:   enqueueTime,
	}
}

func makeEvent(auditID, verb, resource string) auditv1.Event {
	return auditv1.Event{
		AuditID: types.UID(auditID),
		Verb:    verb,
		ObjectRef: &auditv1.ObjectReference{
			Resource: resource,
		},
	}
}

// collectEvents reads up to wantN events from ch within the given timeout.
func collectEvents(ch <-chan auditv1.Event, wantN int, timeout time.Duration) []auditv1.Event {
	var received []auditv1.Event
	deadline := time.After(timeout)
	for {
		select {
		case event, ok := <-ch:
			if !ok {
				return received
			}
			received = append(received, event)
			if len(received) >= wantN {
				return received
			}
		case <-deadline:
			return received
		}
	}
}

// drainChannel consumes remaining events until the channel is closed.
func drainChannel(ch <-chan auditv1.Event) {
	for range ch {
		// drain remaining events until channel closes
	}
}

// assertPartitions checks that the checkpoint contains the expected partition offsets.
func assertPartitions(t *testing.T, cp CloudPosition, want map[string]string) {
	t.Helper()
	for partition, wantSeq := range want {
		gotSeq, ok := cp.PartitionOffsets[partition]
		if !ok {
			t.Errorf("missing partition %q in checkpoint", partition)
			continue
		}
		if gotSeq != wantSeq {
			t.Errorf("partition %q: got seq %q, want %q", partition, gotSeq, wantSeq)
		}
	}
}

func TestCloudIngestor(t *testing.T) {
	tests := []struct {
		name           string
		batches        [][]Message
		parser         EnvelopeParser
		validator      *ClusterIdentityValidator
		wantEvents     int
		wantAckedCount int
		wantPartitions map[string]string
		wantTimestamp  string
	}{
		{
			name: "single batch with one event",
			batches: [][]Message{
				{makeMessage("0", "100", "2026-01-01T00:00:00Z",
					makeEvent("a1", "get", "pods"))},
			},
			parser:         &fakeParser{},
			wantEvents:     1,
			wantAckedCount: 1,
			wantPartitions: map[string]string{"0": "100"},
			wantTimestamp:  "2026-01-01T00:00:00Z",
		},
		{
			name: "single message with multiple events",
			batches: [][]Message{
				{makeMessage("0", "200", "2026-01-02T00:00:00Z",
					makeEvent("a1", "get", "pods"),
					makeEvent("a2", "list", "deployments"),
					makeEvent("a3", "create", "secrets"))},
			},
			parser:         &fakeParser{},
			wantEvents:     3,
			wantAckedCount: 1,
			wantPartitions: map[string]string{"0": "200"},
			wantTimestamp:  "2026-01-02T00:00:00Z",
		},
		{
			name: "multiple batches across partitions",
			batches: [][]Message{
				{makeMessage("0", "10", "2026-01-01T00:00:00Z",
					makeEvent("a1", "get", "pods"))},
				{makeMessage("1", "20", "2026-01-01T00:01:00Z",
					makeEvent("a2", "list", "services"))},
			},
			parser:         &fakeParser{},
			wantEvents:     2,
			wantAckedCount: 2,
			wantPartitions: map[string]string{"0": "10", "1": "20"},
			wantTimestamp:  "2026-01-01T00:01:00Z",
		},
		{
			name: "malformed message skipped",
			batches: [][]Message{
				{
					{Body: []byte("not json"), Partition: "0", SequenceNumber: "1"},
					makeMessage("0", "2", "2026-01-01T00:00:00Z",
						makeEvent("a1", "get", "pods")),
				},
			},
			parser:         &fakeParser{},
			wantEvents:     1,
			wantAckedCount: 1,
			wantPartitions: map[string]string{"0": "2"},
		},
		{
			name: "empty events in message",
			batches: [][]Message{
				{makeMessage("0", "1", "2026-01-01T00:00:00Z")},
			},
			parser:         &fakeParser{},
			wantEvents:     0,
			wantAckedCount: 1,
			wantPartitions: map[string]string{"0": "1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := NewFakeSource(tt.batches...)
			ing := NewCloudIngestor(source, tt.parser, tt.validator, CloudPosition{}, "test")

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			ch, err := ing.Start(ctx)
			if err != nil {
				t.Fatalf("Start() error = %v", err)
			}

			received := collectEvents(ch, tt.wantEvents, 3*time.Second)
			cancel()
			drainChannel(ch)

			if len(received) != tt.wantEvents {
				t.Errorf("got %d events, want %d", len(received), tt.wantEvents)
			}

			acked := source.AckedBatches()
			if len(acked) != tt.wantAckedCount {
				t.Errorf("got %d ack batches, want %d", len(acked), tt.wantAckedCount)
			}

			cp := ing.CloudCheckpoint()
			assertPartitions(t, cp, tt.wantPartitions)

			if tt.wantTimestamp != "" && cp.LastTimestamp != tt.wantTimestamp {
				t.Errorf("LastTimestamp = %q, want %q", cp.LastTimestamp, tt.wantTimestamp)
			}

			if !source.Closed() {
				t.Error("source was not closed after context cancellation")
			}
		})
	}
}

func TestCloudIngestor_ContextCancellation(t *testing.T) {
	// Source that never returns messages — tests clean shutdown.
	source := NewFakeSource() // no batches
	ing := NewCloudIngestor(source, &fakeParser{}, nil, CloudPosition{}, "test")

	ctx, cancel := context.WithCancel(context.Background())

	ch, err := ing.Start(ctx)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Cancel immediately.
	cancel()

	// Channel should close.
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("expected channel to be closed")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("channel did not close within timeout")
	}

	if !source.Closed() {
		t.Error("source was not closed")
	}
}

func TestCloudIngestor_ConnectError(t *testing.T) {
	source := NewFakeSource()
	source.ConnectErr = context.DeadlineExceeded

	ing := NewCloudIngestor(source, &fakeParser{}, nil, CloudPosition{}, "test")

	_, err := ing.Start(context.Background())
	if err == nil {
		t.Fatal("expected error from Start()")
	}
}

func TestCloudIngestor_CheckpointRestore(t *testing.T) {
	startPos := CloudPosition{
		PartitionOffsets: map[string]string{"0": "50", "1": "25"},
		LastTimestamp:    "2026-01-01T00:00:00Z",
	}

	source := NewFakeSource(
		[]Message{makeMessage("0", "51", "2026-01-01T01:00:00Z",
			makeEvent("a1", "get", "pods"))},
	)

	ing := NewCloudIngestor(source, &fakeParser{}, nil, startPos, "test")

	// Verify initial checkpoint is the restored position.
	cp := ing.CloudCheckpoint()
	if cp.PartitionOffsets["0"] != "50" {
		t.Errorf("initial partition 0: got %q, want %q", cp.PartitionOffsets["0"], "50")
	}
	if cp.PartitionOffsets["1"] != "25" {
		t.Errorf("initial partition 1: got %q, want %q", cp.PartitionOffsets["1"], "25")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	ch, err := ing.Start(ctx)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Consume the event.
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for event")
	}

	cancel()
	drainChannel(ch)

	// Verify checkpoint was updated for partition 0 but preserved for partition 1.
	cp = ing.CloudCheckpoint()
	if cp.PartitionOffsets["0"] != "51" {
		t.Errorf("updated partition 0: got %q, want %q", cp.PartitionOffsets["0"], "51")
	}
	if cp.PartitionOffsets["1"] != "25" {
		t.Errorf("preserved partition 1: got %q, want %q", cp.PartitionOffsets["1"], "25")
	}
	if cp.LastTimestamp != "2026-01-01T01:00:00Z" {
		t.Errorf("LastTimestamp = %q, want %q", cp.LastTimestamp, "2026-01-01T01:00:00Z")
	}
}

func TestCloudIngestor_CheckpointRestoredToSource(t *testing.T) {
	// This test verifies that Start() passes the saved checkpoint to sources
	// that implement CheckpointRestorer, and does so BEFORE calling Connect().
	// Without this, pull-based sources (CloudWatch) would start from a default
	// lookback instead of the saved checkpoint after a restart.
	startPos := CloudPosition{
		PartitionOffsets: map[string]string{"0": "50"},
		LastTimestamp:    "2026-01-01T00:00:00Z",
	}

	source := NewFakeCheckpointSource(
		[]Message{makeMessage("0", "51", "2026-01-01T01:00:00Z",
			makeEvent("a1", "get", "pods"))},
	)

	ing := NewCloudIngestor(source, &fakeParser{}, nil, startPos, "test")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	ch, err := ing.Start(ctx)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for event")
	}

	cancel()
	drainChannel(ch)

	// Verify RestoreCheckpoint was called with the right position.
	restored := source.RestoredPosition()
	if restored == nil {
		t.Fatal("RestoreCheckpoint was never called")
	}
	if restored.LastTimestamp != "2026-01-01T00:00:00Z" {
		t.Errorf("restored LastTimestamp = %q, want %q", restored.LastTimestamp, "2026-01-01T00:00:00Z")
	}
	if restored.PartitionOffsets["0"] != "50" {
		t.Errorf("restored partition 0 = %q, want %q", restored.PartitionOffsets["0"], "50")
	}

	// Verify RestoreCheckpoint was called BEFORE Connect.
	if !source.RestoreCalledBeforeConnect() {
		t.Error("RestoreCheckpoint must be called before Connect, but was called after")
	}
}

func TestCloudIngestor_NoCheckpointRestore_WithoutInterface(t *testing.T) {
	// Verify that sources NOT implementing CheckpointRestorer (like Azure's
	// EventHubSource) still work fine — Start() simply skips the restore call.
	source := NewFakeSource(
		[]Message{makeMessage("0", "1", "2026-01-01T00:00:00Z",
			makeEvent("a1", "get", "pods"))},
	)

	startPos := CloudPosition{
		LastTimestamp: "2025-12-31T00:00:00Z",
	}

	ing := NewCloudIngestor(source, &fakeParser{}, nil, startPos, "test")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	ch, err := ing.Start(ctx)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for event")
	}

	cancel()
	drainChannel(ch)

	// Should still work fine — no panic, no error.
}

func TestCloudIngestor_PositionAdapter(t *testing.T) {
	// Verify Checkpoint() returns ingestor.Position with LastTimestamp.
	source := NewFakeSource(
		[]Message{makeMessage("0", "1", "2026-06-15T12:00:00Z",
			makeEvent("a1", "get", "pods"))},
	)

	ing := NewCloudIngestor(source, &fakeParser{}, nil, CloudPosition{}, "test")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	ch, err := ing.Start(ctx)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for event")
	}

	cancel()
	drainChannel(ch)

	pos := ing.Checkpoint()
	if pos.LastTimestamp != "2026-06-15T12:00:00Z" {
		t.Errorf("Position.LastTimestamp = %q, want %q", pos.LastTimestamp, "2026-06-15T12:00:00Z")
	}
	if pos.FileOffset != 0 {
		t.Errorf("Position.FileOffset = %d, want 0", pos.FileOffset)
	}
	if pos.Inode != 0 {
		t.Errorf("Position.Inode = %d, want 0", pos.Inode)
	}
}
