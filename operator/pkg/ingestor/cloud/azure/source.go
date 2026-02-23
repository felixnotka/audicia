//go:build azure

package azure

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azeventhubs/v2"
	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azeventhubs/v2/checkpoints"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/container"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/felixnotka/audicia/operator/pkg/ingestor/cloud"
)

var log = ctrl.Log.WithName("ingestor").WithName("cloud").WithName("azure")

// EventHubSource implements cloud.MessageSource using the Azure Event Hub Processor.
// It uses the load-balanced Processor pattern for distributed consumption.
type EventHubSource struct {
	Namespace     string // Fully qualified namespace (e.g., "myns.servicebus.windows.net")
	EventHub      string
	ConsumerGroup string
	ConnectionStr string // Optional: if set, use connection string instead of managed identity.

	// StorageAccountURL and StorageContainerName configure checkpoint blob storage.
	// If empty, no external checkpoint store is used (in-memory only).
	StorageAccountURL    string
	StorageContainerName string

	mu              sync.Mutex
	consumerClient  *azeventhubs.ConsumerClient
	processor       *azeventhubs.Processor
	partitionClient *azeventhubs.ProcessorPartitionClient
	lastEvents      []*azeventhubs.ReceivedEventData
	processorCancel context.CancelFunc
	processorDone   chan struct{}
}

func (s *EventHubSource) Connect(ctx context.Context) error {
	var (
		client *azeventhubs.ConsumerClient
		err    error
	)

	consumerGroup := s.ConsumerGroup
	if consumerGroup == "" {
		consumerGroup = azeventhubs.DefaultConsumerGroup
	}

	if s.ConnectionStr != "" {
		client, err = azeventhubs.NewConsumerClientFromConnectionString(
			s.ConnectionStr, s.EventHub, consumerGroup, nil)
	} else {
		cred, credErr := azidentity.NewDefaultAzureCredential(nil)
		if credErr != nil {
			return fmt.Errorf("creating Azure credential: %w", credErr)
		}
		client, err = azeventhubs.NewConsumerClient(
			s.Namespace, s.EventHub, consumerGroup, cred, nil)
	}
	if err != nil {
		return fmt.Errorf("creating Event Hub consumer client: %w", err)
	}

	checkpointStore, err := s.buildCheckpointStore(ctx)
	if err != nil {
		client.Close(ctx)
		return fmt.Errorf("creating checkpoint store: %w", err)
	}

	processor, err := azeventhubs.NewProcessor(client, checkpointStore, &azeventhubs.ProcessorOptions{
		// Balanced: claim one partition per interval until balanced across instances.
		LoadBalancingStrategy: azeventhubs.ProcessorStrategyBalanced,
		StartPositions: azeventhubs.StartPositions{
			Default: azeventhubs.StartPosition{
				Earliest: ptrBool(true),
			},
		},
	})
	if err != nil {
		client.Close(ctx)
		return fmt.Errorf("creating Event Hub processor: %w", err)
	}

	s.mu.Lock()
	s.consumerClient = client
	s.processor = processor
	s.mu.Unlock()

	// Run the processor in the background. It handles partition ownership and load balancing.
	processorCtx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	s.mu.Lock()
	s.processorCancel = cancel
	s.processorDone = done
	s.mu.Unlock()

	go func() {
		defer close(done)
		if err := processor.Run(processorCtx); err != nil {
			log.Error(err, "Event Hub processor exited with error")
		}
	}()

	// Dispatch partition clients in a background goroutine.
	// Each acquired partition is served sequentially through Receive().
	go s.dispatchPartitions(processorCtx)

	log.Info("connected to Event Hub",
		"namespace", s.Namespace, "eventHub", s.EventHub, "consumerGroup", consumerGroup)
	return nil
}

func (s *EventHubSource) Receive(ctx context.Context) ([]cloud.Message, error) {
	// Wait for a partition client to be available.
	s.mu.Lock()
	pc := s.partitionClient
	s.mu.Unlock()

	if pc == nil {
		// No partition client yet — wait for one to be assigned.
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(1 * time.Second):
			return nil, nil // retry loop in CloudIngestor will call again
		}
	}

	receiveCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	events, err := pc.ReceiveEvents(receiveCtx, 100, nil)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		// Check for ownership lost — partition was reassigned to another instance.
		var ehErr *azeventhubs.Error
		if errors.As(err, &ehErr) && ehErr.Code == azeventhubs.ErrorCodeOwnershipLost {
			log.V(1).Info("partition ownership lost, will acquire new partition",
				"partition", pc.PartitionID())
			s.mu.Lock()
			s.partitionClient = nil
			s.mu.Unlock()
			pc.Close(ctx)
			return nil, nil
		}
		// Timeout is normal — no events available.
		if receiveCtx.Err() != nil {
			return nil, nil
		}
		return nil, fmt.Errorf("receiving events from partition %s: %w", pc.PartitionID(), err)
	}

	if len(events) == 0 {
		return nil, nil
	}

	msgs := make([]cloud.Message, 0, len(events))
	for _, e := range events {
		msgs = append(msgs, cloud.Message{
			Body:           e.Body,
			SequenceNumber: strconv.FormatInt(e.SequenceNumber, 10),
			Partition:      pc.PartitionID(),
			EnqueuedTime:   e.EnqueuedTime.UTC().Format(time.RFC3339),
		})
	}

	// Store the last event for checkpointing in Acknowledge.
	s.mu.Lock()
	s.lastEvents = events
	s.mu.Unlock()

	return msgs, nil
}

func (s *EventHubSource) Acknowledge(ctx context.Context, msgs []cloud.Message) error {
	s.mu.Lock()
	pc := s.partitionClient
	lastEvents := s.lastEvents
	s.mu.Unlock()

	if pc == nil || len(lastEvents) == 0 {
		return nil
	}

	// Checkpoint at the last received event.
	if err := pc.UpdateCheckpoint(ctx, lastEvents[len(lastEvents)-1], nil); err != nil {
		return fmt.Errorf("updating checkpoint for partition %s: %w", pc.PartitionID(), err)
	}

	log.V(2).Info("checkpointed partition",
		"partition", pc.PartitionID(),
		"sequenceNumber", lastEvents[len(lastEvents)-1].SequenceNumber)
	return nil
}

func (s *EventHubSource) Close(ctx context.Context) error {
	s.mu.Lock()
	cancel := s.processorCancel
	done := s.processorDone
	client := s.consumerClient
	pc := s.partitionClient
	s.mu.Unlock()

	if pc != nil {
		pc.Close(ctx)
	}
	if cancel != nil {
		cancel()
	}
	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
		}
	}
	if client != nil {
		return client.Close(ctx)
	}
	return nil
}

// dispatchPartitions continuously acquires partition clients from the processor.
func (s *EventHubSource) dispatchPartitions(ctx context.Context) {
	for {
		pc := s.processor.NextPartitionClient(ctx)
		if pc == nil {
			return // processor stopped
		}

		log.Info("acquired partition", "partition", pc.PartitionID())

		// Wait until the current partition client is consumed (nil) before assigning a new one.
		for {
			s.mu.Lock()
			current := s.partitionClient
			s.mu.Unlock()

			if current == nil {
				break
			}

			select {
			case <-ctx.Done():
				pc.Close(ctx)
				return
			case <-time.After(500 * time.Millisecond):
			}
		}

		s.mu.Lock()
		s.partitionClient = pc
		s.mu.Unlock()
	}
}

func (s *EventHubSource) buildCheckpointStore(ctx context.Context) (azeventhubs.CheckpointStore, error) {
	if s.StorageAccountURL == "" || s.StorageContainerName == "" {
		// No external checkpoint store configured — use in-memory.
		// Checkpoints are persisted in AudiciaSource status instead.
		return newInMemoryCheckpointStore(), nil
	}

	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("creating credential for checkpoint store: %w", err)
	}

	blobURL := fmt.Sprintf("%s/%s", s.StorageAccountURL, s.StorageContainerName)
	containerClient, err := container.NewClient(blobURL, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("creating blob container client: %w", err)
	}

	store, err := checkpoints.NewBlobStore(containerClient, nil)
	if err != nil {
		return nil, fmt.Errorf("creating blob checkpoint store: %w", err)
	}
	return store, nil
}

func ptrBool(b bool) *bool { return &b }
