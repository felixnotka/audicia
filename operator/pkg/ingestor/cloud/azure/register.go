//go:build azure

package azure

import (
	"fmt"

	audiciav1alpha1 "github.com/felixnotka/audicia/operator/pkg/apis/audicia.io/v1alpha1"
	"github.com/felixnotka/audicia/operator/pkg/ingestor/cloud"
)

func init() {
	cloud.RegisterAdapter(audiciav1alpha1.CloudProviderAzureEventHub, buildAzureAdapter)
}

func buildAzureAdapter(cfg *audiciav1alpha1.CloudConfig) (cloud.MessageSource, cloud.EnvelopeParser, error) {
	if cfg.Azure == nil {
		return nil, nil, fmt.Errorf("azure configuration is required for AzureEventHub provider")
	}

	if cfg.Azure.EventHubNamespace == "" {
		return nil, nil, fmt.Errorf("azure.eventHubNamespace is required")
	}
	if cfg.Azure.EventHubName == "" {
		return nil, nil, fmt.Errorf("azure.eventHubName is required")
	}

	source := &EventHubSource{
		Namespace:            cfg.Azure.EventHubNamespace,
		EventHub:             cfg.Azure.EventHubName,
		ConsumerGroup:        cfg.Azure.ConsumerGroup,
		StorageAccountURL:    cfg.Azure.StorageAccountURL,
		StorageContainerName: cfg.Azure.StorageContainerName,
	}

	// CredentialSecretName handling is done by the controller which reads the
	// secret and sets ConnectionStr before Connect() is called. The adapter
	// factory only wires the config fields here.

	return source, &EnvelopeParser{}, nil
}
