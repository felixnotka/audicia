package ingestor

import (
	"context"

	auditv1 "k8s.io/apiserver/pkg/apis/audit/v1"
)

// Ingestor reads audit events from a source and emits them on a channel.
type Ingestor interface {
	// Start begins reading audit events. Events are sent to the returned channel.
	// The channel is closed when the context is cancelled or an error occurs.
	Start(ctx context.Context) (<-chan auditv1.Event, error)

	// Checkpoint returns the current processing position for persistence.
	Checkpoint() Position
}

// Position represents a resumable position in the audit stream.
type Position struct {
	// FileOffset is the byte offset in the audit log file.
	FileOffset int64

	// Inode is the inode number of the file (for rotation detection).
	Inode uint64

	// LastTimestamp is the timestamp of the last processed event.
	LastTimestamp string
}
