//go:build gcp

package gcp

import (
	"context"
	"fmt"
	"sync"
	"time"

	"cloud.google.com/go/pubsub"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/felixnotka/audicia/operator/pkg/ingestor/cloud"
)

var log = ctrl.Log.WithName("ingestor").WithName("cloud").WithName("gcp")

// maxOutstandingMessages limits how many unacked messages the Pub/Sub client
// will hold in memory. This also serves as the msgCh buffer size.
const maxOutstandingMessages = 100

// maxBatchSize is the maximum number of messages returned per Receive() call.
const maxBatchSize = 100

// batchWindow is how long Receive() waits after the first message to collect
// additional messages into the batch before returning.
const batchWindow = 50 * time.Millisecond

// PubSubSource implements cloud.MessageSource using GCP Pub/Sub's streaming
// pull. It bridges the callback-based subscription.Receive() to the
// synchronous MessageSource.Receive() interface via an internal channel.
type PubSubSource struct {
	ProjectID      string
	SubscriptionID string

	mu         sync.Mutex
	client     *pubsub.Client
	sub        *pubsub.Subscription
	msgCh      chan *pubsub.Message       // callback → Receive() bridge
	cancelRecv context.CancelFunc         // cancels the background sub.Receive()
	recvDone   chan struct{}              // closed when background receive exits
	pending    map[string]*pubsub.Message // msg.ID → original for Ack
}

func (s *PubSubSource) Connect(ctx context.Context) error {
	client, err := pubsub.NewClient(ctx, s.ProjectID)
	if err != nil {
		return fmt.Errorf("creating Pub/Sub client: %w", err)
	}

	sub := client.Subscription(s.SubscriptionID)
	sub.ReceiveSettings.MaxOutstandingMessages = maxOutstandingMessages

	s.mu.Lock()
	s.client = client
	s.sub = sub
	s.msgCh = make(chan *pubsub.Message, maxOutstandingMessages)
	s.pending = make(map[string]*pubsub.Message)
	s.mu.Unlock()

	// Start background receive goroutine. Use a detached context so the
	// goroutine lifetime is controlled by cancelRecv, not the caller's ctx.
	recvCtx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	s.mu.Lock()
	s.cancelRecv = cancel
	s.recvDone = done
	s.mu.Unlock()

	go func() {
		defer close(done)
		err := sub.Receive(recvCtx, func(_ context.Context, msg *pubsub.Message) {
			select {
			case s.msgCh <- msg:
			case <-recvCtx.Done():
				msg.Nack()
			}
		})
		if err != nil && recvCtx.Err() == nil {
			log.Error(err, "Pub/Sub receive exited unexpectedly")
		}
		// Close the channel so Receive() unblocks on shutdown.
		close(s.msgCh)
	}()

	log.Info("connected to Pub/Sub",
		"project", s.ProjectID, "subscription", s.SubscriptionID)
	return nil
}

func (s *PubSubSource) Receive(ctx context.Context) ([]cloud.Message, error) {
	s.mu.Lock()
	msgCh := s.msgCh
	s.mu.Unlock()

	if msgCh == nil {
		return nil, fmt.Errorf("Pub/Sub client not connected")
	}

	// Block for the first message.
	var first *pubsub.Message
	select {
	case msg, ok := <-msgCh:
		if !ok {
			return nil, nil // channel closed, clean shutdown
		}
		first = msg
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// Batch additional messages with a short window.
	batch := make([]*pubsub.Message, 0, maxBatchSize)
	batch = append(batch, first)

	timer := time.NewTimer(batchWindow)
	defer timer.Stop()

loop:
	for len(batch) < maxBatchSize {
		select {
		case msg, ok := <-msgCh:
			if !ok {
				break loop // channel closed
			}
			batch = append(batch, msg)
		case <-timer.C:
			break loop
		case <-ctx.Done():
			// Nack all collected messages before returning.
			for _, m := range batch {
				m.Nack()
			}
			return nil, ctx.Err()
		}
	}

	// Store originals for later Ack and convert to cloud.Message.
	s.mu.Lock()
	msgs := make([]cloud.Message, 0, len(batch))
	for _, msg := range batch {
		s.pending[msg.ID] = msg
		msgs = append(msgs, cloud.Message{
			Body:           msg.Data,
			SequenceNumber: msg.ID,
			Partition:      s.SubscriptionID,
			EnqueuedTime:   msg.PublishTime.UTC().Format(time.RFC3339),
		})
	}
	s.mu.Unlock()

	return msgs, nil
}

func (s *PubSubSource) Acknowledge(_ context.Context, msgs []cloud.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, msg := range msgs {
		if original, ok := s.pending[msg.SequenceNumber]; ok {
			original.Ack()
			delete(s.pending, msg.SequenceNumber)
		}
	}

	return nil
}

func (s *PubSubSource) Close(ctx context.Context) error {
	s.mu.Lock()
	cancel := s.cancelRecv
	done := s.recvDone
	client := s.client
	pending := s.pending
	s.pending = nil
	s.client = nil
	s.mu.Unlock()

	// Nack pending messages so Pub/Sub redelivers them.
	for _, msg := range pending {
		msg.Nack()
	}

	if cancel != nil {
		cancel()
	}

	// Wait for the background receive to finish.
	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
		}
	}

	if client != nil {
		if err := client.Close(); err != nil {
			return fmt.Errorf("closing Pub/Sub client: %w", err)
		}
	}

	log.Info("closed Pub/Sub source")
	return nil
}
