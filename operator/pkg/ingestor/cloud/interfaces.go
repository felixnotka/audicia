package cloud

import (
	"context"

	auditv1 "k8s.io/apiserver/pkg/apis/audit/v1"
)

// Message is a raw message received from a cloud message bus.
type Message struct {
	// Body is the raw message payload (cloud envelope JSON).
	Body []byte

	// SequenceNumber uniquely identifies this message within the partition/shard.
	SequenceNumber string

	// Partition identifies the source partition (Event Hub partition, CloudWatch shard, etc.).
	Partition string

	// EnqueuedTime is when the cloud provider received the message (RFC3339).
	EnqueuedTime string
}

// MessageSource connects to a cloud message bus and delivers raw messages.
type MessageSource interface {
	// Connect establishes the connection to the cloud message bus.
	Connect(ctx context.Context) error

	// Receive returns the next batch of messages. Blocks until messages are
	// available or the context is cancelled. Returns nil, nil on clean shutdown.
	Receive(ctx context.Context) ([]Message, error)

	// Acknowledge marks messages as successfully processed, advancing the
	// cloud-side checkpoint (e.g., Event Hub checkpoint store).
	Acknowledge(ctx context.Context, msgs []Message) error

	// Close releases resources held by the source.
	Close(ctx context.Context) error
}

// EnvelopeParser unwraps a cloud-provider-specific message envelope and
// extracts the Kubernetes audit events it contains.
type EnvelopeParser interface {
	// Parse extracts zero or more audit events from the raw message body.
	// A single cloud message may contain multiple audit events (batch).
	// Returns nil (not error) for messages that don't contain audit events
	// (e.g., diagnostic metadata messages in Event Hub).
	Parse(body []byte) ([]auditv1.Event, error)
}

// CheckpointRestorer is an optional interface that a MessageSource can
// implement to restore internal state from a saved CloudPosition before
// Connect() is called. Pull-based sources (e.g., CloudWatch) use this to
// set their start offset on restart. Push-based sources (e.g., Event Hub)
// manage their own checkpoint stores and don't need this.
type CheckpointRestorer interface {
	RestoreCheckpoint(pos CloudPosition)
}
