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
