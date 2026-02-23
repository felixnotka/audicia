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
	Parser   EnvelopeParser
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
		LastTimestamp:     c.position.LastTimestamp,
		PartitionOffsets: make(map[string]string, len(c.position.PartitionOffsets)),
	}
	for k, v := range c.position.PartitionOffsets {
		cp.PartitionOffsets[k] = v
	}
	return cp
}

func (c *CloudIngestor) receiveLoop(ctx context.Context, ch chan<- auditv1.Event) {
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := c.Source.Close(closeCtx); err != nil {
			cloudLog.Error(err, "error closing cloud message source")
		}
		close(ch)
	}()

	for {
		msgs, err := c.Source.Receive(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return // context cancelled, clean shutdown
			}
			metrics.CloudReceiveErrorsTotal.WithLabelValues(c.ProviderLabel).Inc()
			cloudLog.Error(err, "error receiving messages, retrying in 5s")
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
				continue
			}
		}
		if len(msgs) == 0 {
			continue
		}

		// Track per-partition message counts.
		partitionCounts := map[string]int{}
		for _, msg := range msgs {
			partitionCounts[msg.Partition]++
		}
		for partition, count := range partitionCounts {
			metrics.CloudMessagesReceivedTotal.WithLabelValues(c.ProviderLabel, partition).Add(float64(count))
		}

		// Observe lag from the last message in the batch.
		last := msgs[len(msgs)-1]
		if last.EnqueuedTime != "" {
			if enqueued, parseErr := time.Parse(time.RFC3339, last.EnqueuedTime); parseErr == nil {
				lag := time.Since(enqueued).Seconds()
				metrics.CloudLagSeconds.WithLabelValues(c.ProviderLabel).Observe(lag)
			}
		}

		var emitted int
		for _, msg := range msgs {
			events, err := c.Parser.Parse(msg.Body)
			if err != nil {
				metrics.CloudEnvelopeParseErrorsTotal.WithLabelValues(c.ProviderLabel).Inc()
				cloudLog.V(1).Info("skipping unparseable message",
					"error", err, "partition", msg.Partition, "seq", msg.SequenceNumber)
				continue
			}

			for _, event := range events {
				if c.Validator != nil && !c.Validator.Matches(event) {
					cloudLog.V(2).Info("dropping event from different cluster",
						"auditID", event.AuditID)
					continue
				}

				select {
				case ch <- event:
					emitted++
				case <-ctx.Done():
					return
				}
			}
		}

		// Acknowledge all messages in this batch after successful processing.
		if err := c.Source.Acknowledge(ctx, msgs); err != nil {
			if ctx.Err() != nil {
				return
			}
			cloudLog.Error(err, "failed to acknowledge messages")
		} else {
			metrics.CloudMessagesAckedTotal.WithLabelValues(c.ProviderLabel).Inc()
		}

		// Update checkpoint from the last message in the batch.
		c.updatePosition(last)

		cloudLog.V(1).Info("processed batch",
			"messages", len(msgs), "events", emitted)
	}
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
