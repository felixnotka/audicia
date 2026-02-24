//go:build aws

package main

// Register the AWS CloudWatch adapter. The init() function in the aws
// package calls cloud.RegisterAdapter(), making the AWSCloudWatch provider
// available to the cloud ingestor.
import _ "github.com/felixnotka/audicia/operator/pkg/ingestor/cloud/aws"
