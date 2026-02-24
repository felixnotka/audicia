//go:build aws

package aws

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/felixnotka/audicia/operator/pkg/ingestor/cloud"
)

var log = ctrl.Log.WithName("ingestor").WithName("cloud").WithName("aws")

// pollInterval is the time to wait between FilterLogEvents calls when all
// pages have been consumed and no new events are available.
const pollInterval = 5 * time.Second

// defaultLookback is how far back to start reading when there is no checkpoint.
const defaultLookback = 5 * time.Minute

// maxEventsPerPage is the maximum number of events returned per FilterLogEvents call.
const maxEventsPerPage = 100

// CloudWatchSource implements cloud.MessageSource using the AWS CloudWatch
// Logs FilterLogEvents API. It polls for new audit log events and converts
// them to cloud.Message for the shared CloudIngestor receive loop.
type CloudWatchSource struct {
	LogGroupName    string
	LogStreamPrefix string
	Region          string // Optional: if empty, uses AWS_REGION from environment.

	mu        sync.Mutex
	client    *cloudwatchlogs.Client
	startTime int64  // Millis since epoch — exclusive lower bound for FilterLogEvents.
	nextToken *string
}

func (s *CloudWatchSource) Connect(ctx context.Context) error {
	var opts []func(*config.LoadOptions) error

	if s.Region != "" {
		opts = append(opts, config.WithRegion(s.Region))
	}

	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return fmt.Errorf("loading AWS config: %w", err)
	}

	s.mu.Lock()
	s.client = cloudwatchlogs.NewFromConfig(cfg)
	if s.startTime == 0 {
		s.startTime = time.Now().Add(-defaultLookback).UnixMilli()
	}
	s.mu.Unlock()

	log.Info("connected to CloudWatch Logs",
		"logGroup", s.LogGroupName, "region", cfg.Region)
	return nil
}

func (s *CloudWatchSource) Receive(ctx context.Context) ([]cloud.Message, error) {
	s.mu.Lock()
	client := s.client
	startTime := s.startTime
	nextToken := s.nextToken
	s.mu.Unlock()

	if client == nil {
		return nil, fmt.Errorf("CloudWatch client not connected")
	}

	input := &cloudwatchlogs.FilterLogEventsInput{
		LogGroupName: aws.String(s.LogGroupName),
		StartTime:    aws.Int64(startTime),
		Limit:        aws.Int32(maxEventsPerPage),
	}

	if s.LogStreamPrefix != "" {
		input.LogStreamNamePrefix = aws.String(s.LogStreamPrefix)
	}

	if nextToken != nil {
		input.NextToken = nextToken
	}

	resp, err := client.FilterLogEvents(ctx, input)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("FilterLogEvents: %w", err)
	}

	// Update pagination and startTime atomically.
	//
	// startTime is only advanced when pagination completes (NextToken == nil)
	// to avoid skipping events at page boundaries. FilterLogEvents returns
	// events sorted ascending by Timestamp, so the last event in the final
	// page has the highest timestamp across all pages. We advance startTime
	// past it so the next poll cycle starts fresh.
	//
	// If we advanced startTime after each page (like in Acknowledge), page 2
	// events sharing the same millisecond as page 1's last event would be
	// skipped because startTime is exclusive (lastTimestamp + 1).
	s.mu.Lock()
	s.nextToken = resp.NextToken
	if resp.NextToken == nil && len(resp.Events) > 0 {
		last := resp.Events[len(resp.Events)-1]
		if last.Timestamp != nil {
			s.startTime = aws.ToInt64(last.Timestamp) + 1
		}
	}
	s.mu.Unlock()

	if len(resp.Events) == 0 {
		if resp.NextToken == nil {
			// All pages consumed — wait before polling again.
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(pollInterval):
			}
		}
		return nil, nil
	}

	msgs := make([]cloud.Message, 0, len(resp.Events))
	for _, event := range resp.Events {
		msgs = append(msgs, convertEvent(event))
	}

	return msgs, nil
}

// convertEvent converts a CloudWatch FilteredLogEvent to a cloud.Message.
//
// EnqueuedTime uses the event's Timestamp (when the event occurred), NOT
// IngestionTime (when CloudWatch received it). This is critical because
// FilterLogEvents.startTime filters on Timestamp. If we checkpointed based
// on IngestionTime but filtered on Timestamp, we could miss events where
// Timestamp < IngestionTime (which is the common case — there is always
// some delay between event creation and CloudWatch ingestion).
func convertEvent(event types.FilteredLogEvent) cloud.Message {
	msg := cloud.Message{
		Body: []byte(aws.ToString(event.Message)),
	}

	if event.EventId != nil {
		msg.SequenceNumber = aws.ToString(event.EventId)
	}

	if event.LogStreamName != nil {
		msg.Partition = aws.ToString(event.LogStreamName)
	}

	if event.Timestamp != nil {
		msg.EnqueuedTime = time.UnixMilli(aws.ToInt64(event.Timestamp)).UTC().Format(time.RFC3339)
	}

	return msg
}

func (s *CloudWatchSource) Acknowledge(_ context.Context, _ []cloud.Message) error {
	// CloudWatch Logs is pull-based — no message acknowledgment needed.
	// startTime advancement is handled in Receive() when pagination completes,
	// and persistent checkpoint tracking is handled by CloudIngestor.updatePosition().
	return nil
}

func (s *CloudWatchSource) Close(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.client = nil
	s.nextToken = nil
	log.Info("closed CloudWatch Logs source")
	return nil
}

// RestoreCheckpoint implements cloud.CheckpointRestorer. It sets the
// startTime from the saved CloudPosition so that polling resumes from
// the last acknowledged event after a restart.
func (s *CloudWatchSource) RestoreCheckpoint(pos cloud.CloudPosition) {
	if pos.LastTimestamp == "" {
		return
	}
	t, err := time.Parse(time.RFC3339, pos.LastTimestamp)
	if err != nil {
		log.V(1).Info("failed to parse checkpoint timestamp", "timestamp", pos.LastTimestamp, "error", err)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.startTime = t.UnixMilli() + 1 // Start after the last processed event.
	log.Info("restored checkpoint", "startTime", s.startTime, "from", pos.LastTimestamp)
}
