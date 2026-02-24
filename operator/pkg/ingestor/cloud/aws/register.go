//go:build aws

package aws

import (
	"fmt"

	audiciav1alpha1 "github.com/felixnotka/audicia/operator/pkg/apis/audicia.io/v1alpha1"
	"github.com/felixnotka/audicia/operator/pkg/ingestor/cloud"
)

func init() {
	cloud.RegisterAdapter(audiciav1alpha1.CloudProviderAWSCloudWatch, buildAWSAdapter)
}

func buildAWSAdapter(cfg *audiciav1alpha1.CloudConfig) (cloud.MessageSource, cloud.EnvelopeParser, error) {
	if cfg.AWS == nil {
		return nil, nil, fmt.Errorf("aws configuration is required for AWSCloudWatch provider")
	}

	if cfg.AWS.LogGroupName == "" {
		return nil, nil, fmt.Errorf("aws.logGroupName is required")
	}

	source := &CloudWatchSource{
		LogGroupName:    cfg.AWS.LogGroupName,
		LogStreamPrefix: cfg.AWS.LogStreamPrefix,
		Region:          cfg.AWS.Region,
	}

	return source, &EnvelopeParser{}, nil
}
