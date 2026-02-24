//go:build gcp

package main

// Register the GCP Pub/Sub adapter. The init() function in the gcp
// package calls cloud.RegisterAdapter(), making the GCPPubSub provider
// available to the cloud ingestor.
import _ "github.com/felixnotka/audicia/operator/pkg/ingestor/cloud/gcp"
