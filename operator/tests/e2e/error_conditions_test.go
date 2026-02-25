//go:build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	audiciav1alpha1 "github.com/felixnotka/audicia/operator/pkg/apis/audicia.io/v1alpha1"
)

// TestErrorCondition_BadFilterRegex verifies that the operator handles an
// AudiciaSource with an invalid regex in the filter chain gracefully: no crash,
// PipelineStarting condition is set, and no reports are produced.
func TestErrorCondition_BadFilterRegex(t *testing.T) {
	ctx := context.Background()
	suffix := uniqueSuffix()
	ns := "e2e-errfilter-" + suffix

	createNamespace(ctx, t, ns)

	spec := &audiciav1alpha1.AudiciaSourceSpec{
		Filters: []audiciav1alpha1.Filter{
			{
				Action:           audiciav1alpha1.FilterActionDeny,
				NamespacePattern: "[invalid(regex",
			},
		},
	}
	createAudiciaSource(ctx, t, "bad-filter-source", ns, spec)

	// Wait for the operator to reconcile and set the initial condition.
	src := waitForSource(ctx, t, "bad-filter-source", ns, func(s *audiciav1alpha1.AudiciaSource) bool {
		return len(s.Status.Conditions) > 0
	}, 30*time.Second)

	// Pipeline should have started (condition set) but failed due to bad regex.
	// The Ready condition should be PipelineStarting (pipeline goroutine failed silently).
	assertCondition(t, src.Status.Conditions, "Ready", "PipelineStarting", metav1.ConditionFalse)

	// Verify no reports were created (pipeline never reached processing).
	reports := listPolicyReports(ctx, t, ns)
	if len(reports) != 0 {
		t.Errorf("expected 0 reports for failed pipeline, got %d", len(reports))
	}

	t.Log("bad filter regex test passed: operator handled gracefully")
}

// TestErrorCondition_UnknownSourceType verifies that the operator sets the
// PipelineStarting condition even if the source type is unsupported.
// This test uses the webhook source type as a proxy (which will fail to listen
// on a port since TLS files don't exist in the test environment), but the
// key check is that the operator doesn't panic and sets conditions.
func TestErrorCondition_SourceConditionSet(t *testing.T) {
	ctx := context.Background()
	suffix := uniqueSuffix()
	ns := "e2e-errcond-" + suffix

	createNamespace(ctx, t, ns)

	// Create a webhook source with valid config — the webhook server will
	// fail to start (no TLS files on disk), but Reconcile() should still
	// set the PipelineStarting condition before the goroutine fails.
	spec := &audiciav1alpha1.AudiciaSourceSpec{
		SourceType: audiciav1alpha1.SourceTypeWebhook,
		Webhook: &audiciav1alpha1.WebhookConfig{
			Port:          9999,
			TLSSecretName: "nonexistent-tls",
		},
	}
	createAudiciaSource(ctx, t, "errcond-source", ns, spec)

	// Wait for the condition.
	src := waitForSource(ctx, t, "errcond-source", ns, func(s *audiciav1alpha1.AudiciaSource) bool {
		return len(s.Status.Conditions) > 0
	}, 30*time.Second)

	// Should have PipelineStarting (set synchronously in Reconcile).
	assertCondition(t, src.Status.Conditions, "Ready", "PipelineStarting", metav1.ConditionFalse)

	t.Log("source condition test passed")
}
