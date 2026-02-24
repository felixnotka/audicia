//go:build gcp

package gcp

import (
	"fmt"

	audiciav1alpha1 "github.com/felixnotka/audicia/operator/pkg/apis/audicia.io/v1alpha1"
	"github.com/felixnotka/audicia/operator/pkg/ingestor/cloud"
)

func init() {
	cloud.RegisterAdapter(audiciav1alpha1.CloudProviderGCPPubSub, buildGCPAdapter)
}

func buildGCPAdapter(cfg *audiciav1alpha1.CloudConfig) (cloud.MessageSource, cloud.EnvelopeParser, error) {
	if cfg.GCP == nil {
		return nil, nil, fmt.Errorf("gcp configuration is required for GCPPubSub provider")
	}

	if cfg.GCP.ProjectID == "" {
		return nil, nil, fmt.Errorf("gcp.projectID is required")
	}
	if cfg.GCP.SubscriptionID == "" {
		return nil, nil, fmt.Errorf("gcp.subscriptionID is required")
	}

	source := &PubSubSource{
		ProjectID:      cfg.GCP.ProjectID,
		SubscriptionID: cfg.GCP.SubscriptionID,
	}

	return source, &EnvelopeParser{}, nil
}
