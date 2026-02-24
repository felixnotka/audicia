package cloud

import (
	"context"
	"sync"
)

// FakeSource is a test double for MessageSource. It delivers pre-loaded message
// batches and records acknowledgements.
type FakeSource struct {
	mu      sync.Mutex
	batches [][]Message
	acked   [][]Message
	current int
	closed  bool

	// ConnectErr is returned by Connect if set.
	ConnectErr error
}

// NewFakeSource creates a FakeSource with pre-loaded batches.
// Each call to Receive returns the next batch. After all batches are
// exhausted, Receive blocks until the context is cancelled.
func NewFakeSource(batches ...[]Message) *FakeSource {
	return &FakeSource{batches: batches}
}

func (f *FakeSource) Connect(ctx context.Context) error {
	if f.ConnectErr != nil {
		return f.ConnectErr
	}
	return nil
}

func (f *FakeSource) Receive(ctx context.Context) ([]Message, error) {
	f.mu.Lock()
	if f.current < len(f.batches) {
		batch := f.batches[f.current]
		f.current++
		f.mu.Unlock()
		return batch, nil
	}
	f.mu.Unlock()

	// All batches exhausted — block until context cancelled.
	<-ctx.Done()
	return nil, ctx.Err()
}

func (f *FakeSource) Acknowledge(ctx context.Context, msgs []Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.acked = append(f.acked, msgs)
	return nil
}

func (f *FakeSource) Close(ctx context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}

// AckedBatches returns all acknowledged message batches.
func (f *FakeSource) AckedBatches() [][]Message {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.acked
}

// Closed reports whether Close was called.
func (f *FakeSource) Closed() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closed
}

// FakeCheckpointSource extends FakeSource with CheckpointRestorer support.
// It records the restored position so tests can verify that Start() passes
// the saved checkpoint to pull-based sources before calling Connect().
type FakeCheckpointSource struct {
	FakeSource

	mu2             sync.Mutex
	restoredPos     *CloudPosition
	restoreCallTime int // 0 = not called, 1 = before Connect, 2 = after Connect
	connectCalled   bool
}

// NewFakeCheckpointSource creates a FakeCheckpointSource with pre-loaded batches.
func NewFakeCheckpointSource(batches ...[]Message) *FakeCheckpointSource {
	return &FakeCheckpointSource{
		FakeSource: FakeSource{batches: batches},
	}
}

// RestoreCheckpoint implements CheckpointRestorer.
func (f *FakeCheckpointSource) RestoreCheckpoint(pos CloudPosition) {
	f.mu2.Lock()
	defer f.mu2.Unlock()
	posCopy := CloudPosition{
		LastTimestamp:    pos.LastTimestamp,
		PartitionOffsets: make(map[string]string, len(pos.PartitionOffsets)),
	}
	for k, v := range pos.PartitionOffsets {
		posCopy.PartitionOffsets[k] = v
	}
	f.restoredPos = &posCopy
	if f.connectCalled {
		f.restoreCallTime = 2
	} else {
		f.restoreCallTime = 1
	}
}

// Connect overrides FakeSource.Connect to track call ordering.
func (f *FakeCheckpointSource) Connect(ctx context.Context) error {
	f.mu2.Lock()
	f.connectCalled = true
	f.mu2.Unlock()
	return f.FakeSource.Connect(ctx)
}

// RestoredPosition returns the checkpoint position that was passed to
// RestoreCheckpoint, or nil if it was never called.
func (f *FakeCheckpointSource) RestoredPosition() *CloudPosition {
	f.mu2.Lock()
	defer f.mu2.Unlock()
	return f.restoredPos
}

// RestoreCalledBeforeConnect returns true if RestoreCheckpoint was called
// before Connect, which is the correct ordering.
func (f *FakeCheckpointSource) RestoreCalledBeforeConnect() bool {
	f.mu2.Lock()
	defer f.mu2.Unlock()
	return f.restoreCallTime == 1
}
