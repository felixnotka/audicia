//go:build azure

package main

// Register the Azure Event Hub adapter. The init() function in the azure
// package calls cloud.RegisterAdapter(), making the AzureEventHub provider
// available to the cloud ingestor.
import _ "github.com/felixnotka/audicia/operator/pkg/ingestor/cloud/azure"
