//go:build azure

package azure

import (
	"context"
	"sync"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azeventhubs/v2"
)

// inMemoryCheckpointStore implements azeventhubs.CheckpointStore using in-memory maps.
// Used when no Azure Blob Storage is configured — checkpoints are persisted in
// AudiciaSource status instead.
type inMemoryCheckpointStore struct {
	mu          sync.Mutex
	ownerships  map[string]azeventhubs.Ownership
	checkpoints map[string]azeventhubs.Checkpoint
}

func newInMemoryCheckpointStore() *inMemoryCheckpointStore {
	return &inMemoryCheckpointStore{
		ownerships:  make(map[string]azeventhubs.Ownership),
		checkpoints: make(map[string]azeventhubs.Checkpoint),
	}
}

func ownershipKey(o azeventhubs.Ownership) string {
	return o.FullyQualifiedNamespace + "/" + o.EventHubName + "/" + o.ConsumerGroup + "/" + o.PartitionID
}

func checkpointKey(c azeventhubs.Checkpoint) string {
	return c.FullyQualifiedNamespace + "/" + c.EventHubName + "/" + c.ConsumerGroup + "/" + c.PartitionID
}

func (s *inMemoryCheckpointStore) ClaimOwnership(ctx context.Context, partitionOwnership []azeventhubs.Ownership, options *azeventhubs.ClaimOwnershipOptions) ([]azeventhubs.Ownership, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var claimed []azeventhubs.Ownership
	for _, o := range partitionOwnership {
		key := ownershipKey(o)
		s.ownerships[key] = o
		claimed = append(claimed, o)
	}
	return claimed, nil
}

func (s *inMemoryCheckpointStore) ListCheckpoints(ctx context.Context, fullyQualifiedNamespace, eventHubName, consumerGroup string, options *azeventhubs.ListCheckpointsOptions) ([]azeventhubs.Checkpoint, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var result []azeventhubs.Checkpoint
	for _, c := range s.checkpoints {
		if c.FullyQualifiedNamespace == fullyQualifiedNamespace &&
			c.EventHubName == eventHubName &&
			c.ConsumerGroup == consumerGroup {
			result = append(result, c)
		}
	}
	return result, nil
}

func (s *inMemoryCheckpointStore) ListOwnership(ctx context.Context, fullyQualifiedNamespace, eventHubName, consumerGroup string, options *azeventhubs.ListOwnershipOptions) ([]azeventhubs.Ownership, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var result []azeventhubs.Ownership
	for _, o := range s.ownerships {
		if o.FullyQualifiedNamespace == fullyQualifiedNamespace &&
			o.EventHubName == eventHubName &&
			o.ConsumerGroup == consumerGroup {
			result = append(result, o)
		}
	}
	return result, nil
}

func (s *inMemoryCheckpointStore) SetCheckpoint(ctx context.Context, checkpoint azeventhubs.Checkpoint, options *azeventhubs.SetCheckpointOptions) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := checkpointKey(checkpoint)
	s.checkpoints[key] = checkpoint
	return nil
}
