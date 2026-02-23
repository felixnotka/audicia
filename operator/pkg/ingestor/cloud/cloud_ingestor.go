package cloud

import (
	"context"
	"sync"
	"time"

	auditv1 "k8s.io/apiserver/pkg/apis/audit/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/felixnotka/audicia/operator/pkg/ingestor"
	"github.com/felixnotka/audicia/operator/pkg/metrics"
)

var cloudLog = ctrl.Log.WithName("ingestor").WithName("cloud")

// CloudIngestor implements ingestor.Ingestor using a MessageSource and EnvelopeParser.
// It connects to a cloud message bus, receives messages, parses audit events from
// cloud-provider-specific envelopes, and emits them on a channel.
type CloudIngestor struct {
	Source    MessageSource
	Parser    EnvelopeParser
	Validator *ClusterIdentityValidator

	// ProviderLabel is used as the "provider" label in Prometheus metrics.
	ProviderLabel string

	// ChannelBufferSize controls the internal event channel capacity.
	ChannelBufferSize int

	mu       sync.Mutex
	position CloudPosition
}

// NewCloudIngestor creates a cloud-based ingestor.
func NewCloudIngestor(source MessageSource, parser EnvelopeParser, validator *ClusterIdentityValidator, startPos CloudPosition, providerLabel string) *CloudIngestor {
	return &CloudIngestor{
		Source:            source,
		Parser:            parser,
		Validator:         validator,
		ProviderLabel:     providerLabel,
		ChannelBufferSize: 1000,
		position:          startPos,
	}
}

// Start connects to the cloud message bus and begins emitting parsed audit events.
func (c *CloudIngestor) Start(ctx context.Context) (<-chan auditv1.Event, error) {
	if err := c.Source.Connect(ctx); err != nil {
		return nil, err
	}

	ch := make(chan auditv1.Event, c.ChannelBufferSize)
	go c.receiveLoop(ctx, ch)
	return ch, nil
}

// Checkpoint returns the current cloud position adapted to ingestor.Position.
// FileOffset and Inode are zero (not applicable for cloud sources).
func (c *CloudIngestor) Checkpoint() ingestor.Position {
	c.mu.Lock()
	defer c.mu.Unlock()
	return ingestor.Position{
		LastTimestamp: c.position.LastTimestamp,
	}
}

// CloudCheckpoint returns the full cloud-specific checkpoint state
// including per-partition sequence numbers.
func (c *CloudIngestor) CloudCheckpoint() CloudPosition {
	c.mu.Lock()
	defer c.mu.Unlock()
	cp := CloudPosition{
		LastTimestamp:    c.position.LastTimestamp,
		PartitionOffsets: make(map[string]string, len(c.position.PartitionOffsets)),
	}
	for k, v := range c.position.PartitionOffsets {
		cp.PartitionOffsets[k] = v
	}
	return cp
}

func (c *CloudIngestor) receiveLoop(ctx context.Context, ch chan<- auditv1.Event) {
	defer c.closeSource(ch)

	for {
		msgs, err := c.Source.Receive(ctx)
		if err != nil {
			if c.handleReceiveError(ctx, err) {
				return
			}
			continue
		}
		if len(msgs) == 0 {
			continue
		}

		c.recordBatchMetrics(msgs)

		emitted, stopped := c.emitEvents(ctx, ch, msgs)
		if stopped {
			return
		}

		c.acknowledgeBatch(ctx, msgs)
		c.updatePosition(msgs[len(msgs)-1])

		cloudLog.V(1).Info("processed batch",
			"messages", len(msgs), "events", emitted)
	}
}

// closeSource shuts down the cloud message source and closes the event channel.
func (c *CloudIngestor) closeSource(ch chan<- auditv1.Event) {
	closeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := c.Source.Close(closeCtx); err != nil {
		cloudLog.Error(err, "error closing cloud message source")
	}
	close(ch)
}

// handleReceiveError handles a Receive error. Returns true if the loop should exit.
func (c *CloudIngestor) handleReceiveError(ctx context.Context, err error) bool {
	if ctx.Err() != nil {
		return true // context cancelled, clean shutdown
	}
	metrics.CloudReceiveErrorsTotal.WithLabelValues(c.ProviderLabel).Inc()
	cloudLog.Error(err, "error receiving messages, retrying in 5s")
	select {
	case <-ctx.Done():
		return true
	case <-time.After(5 * time.Second):
		return false
	}
}

// recordBatchMetrics records per-partition message counts and consumer lag.
func (c *CloudIngestor) recordBatchMetrics(msgs []Message) {
	partitionCounts := map[string]int{}
	for _, msg := range msgs {
		partitionCounts[msg.Partition]++
	}
	for partition, count := range partitionCounts {
		metrics.CloudMessagesReceivedTotal.WithLabelValues(c.ProviderLabel, partition).Add(float64(count))
	}

	last := msgs[len(msgs)-1]
	if last.EnqueuedTime == "" {
		return
	}
	enqueued, parseErr := time.Parse(time.RFC3339, last.EnqueuedTime)
	if parseErr != nil {
		return
	}
	metrics.CloudLagSeconds.WithLabelValues(c.ProviderLabel).Observe(time.Since(enqueued).Seconds())
}

// emitEvents parses and emits audit events from a message batch.
// Returns the number of emitted events and whether the context was cancelled.
func (c *CloudIngestor) emitEvents(ctx context.Context, ch chan<- auditv1.Event, msgs []Message) (int, bool) {
	var emitted int
	for _, msg := range msgs {
		n, stopped := c.emitMessageEvents(ctx, ch, msg)
		emitted += n
		if stopped {
			return emitted, true
		}
	}
	return emitted, false
}

// emitMessageEvents parses a single message and emits its events to the channel.
func (c *CloudIngestor) emitMessageEvents(ctx context.Context, ch chan<- auditv1.Event, msg Message) (int, bool) {
	events, err := c.Parser.Parse(msg.Body)
	if err != nil {
		metrics.CloudEnvelopeParseErrorsTotal.WithLabelValues(c.ProviderLabel).Inc()
		cloudLog.V(1).Info("skipping unparseable message",
			"error", err, "partition", msg.Partition, "seq", msg.SequenceNumber)
		return 0, false
	}

	var emitted int
	for _, event := range events {
		if c.Validator != nil && !c.Validator.Matches(event) {
			cloudLog.V(2).Info("dropping event from different cluster", "auditID", event.AuditID)
			continue
		}
		select {
		case ch <- event:
			emitted++
		case <-ctx.Done():
			return emitted, true
		}
	}
	return emitted, false
}

// acknowledgeBatch acknowledges a processed message batch.
func (c *CloudIngestor) acknowledgeBatch(ctx context.Context, msgs []Message) {
	if err := c.Source.Acknowledge(ctx, msgs); err != nil {
		if ctx.Err() == nil {
			cloudLog.Error(err, "failed to acknowledge messages")
		}
		return
	}
	metrics.CloudMessagesAckedTotal.WithLabelValues(c.ProviderLabel).Inc()
}

func (c *CloudIngestor) updatePosition(msg Message) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.position.PartitionOffsets == nil {
		c.position.PartitionOffsets = make(map[string]string)
	}
	c.position.PartitionOffsets[msg.Partition] = msg.SequenceNumber
	if msg.EnqueuedTime != "" {
		c.position.LastTimestamp = msg.EnqueuedTime
	}
}
