package cloud

// CloudPosition represents a resumable position in a cloud message stream.
type CloudPosition struct {
	// PartitionOffsets maps partition ID to last-acknowledged sequence number.
	// For Event Hub: partition "0" -> "12345"
	// For CloudWatch: shard "shardId-000" -> "49590..."
	PartitionOffsets map[string]string

	// LastTimestamp is the enqueued time of the last processed message (RFC3339).
	LastTimestamp string
}
