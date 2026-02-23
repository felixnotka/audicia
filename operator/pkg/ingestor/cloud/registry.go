package cloud

import (
	"fmt"

	audiciav1alpha1 "github.com/felixnotka/audicia/operator/pkg/apis/audicia.io/v1alpha1"
)

// AdapterFactory creates a MessageSource and EnvelopeParser pair for a cloud provider.
type AdapterFactory func(cfg *audiciav1alpha1.CloudConfig) (MessageSource, EnvelopeParser, error)

var registry = map[audiciav1alpha1.CloudProvider]AdapterFactory{}

// RegisterAdapter registers an adapter factory for a cloud provider.
// Typically called from an init() function in a provider-specific package.
func RegisterAdapter(provider audiciav1alpha1.CloudProvider, factory AdapterFactory) {
	registry[provider] = factory
}

// BuildAdapter creates the MessageSource and EnvelopeParser for the given config.
func BuildAdapter(cfg *audiciav1alpha1.CloudConfig) (MessageSource, EnvelopeParser, error) {
	factory, ok := registry[cfg.Provider]
	if !ok {
		return nil, nil, fmt.Errorf("unsupported cloud provider: %s (no adapter registered — check build tags)", cfg.Provider)
	}
	return factory(cfg)
}
