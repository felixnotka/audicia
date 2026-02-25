//go:build e2e

package e2e

import (
	"context"
	"testing"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	audiciav1alpha1 "github.com/felixnotka/audicia/operator/pkg/apis/audicia.io/v1alpha1"
)

// TestFilterChain verifies the deny filter prevents events from a specific
// namespace from appearing in the policy report.
func TestFilterChain(t *testing.T) {
	ctx := context.Background()
	suffix := uniqueSuffix()
	allowedNS := "e2e-filter-allowed-" + suffix
	deniedNS := "e2e-filter-denied-" + suffix
	saName := "filter-sa"

	// Setup namespaces and SA.
	createNamespace(ctx, t, allowedNS)
	createNamespace(ctx, t, deniedNS)
	createServiceAccount(ctx, t, saName, allowedNS)

	// Grant SA: list pods in both namespaces.
	podListRule := []rbacv1.PolicyRule{
		{
			APIGroups: []string{""},
			Resources: []string{"pods"},
			Verbs:     []string{"list"},
		},
	}
	createRole(ctx, t, "filter-role-allowed", allowedNS, podListRule)
	bindRoleToSA(ctx, t, "filter-role-allowed", saName, allowedNS, allowedNS, "filter-binding-allowed")

	createRole(ctx, t, "filter-role-denied", deniedNS, podListRule)
	bindRoleToSA(ctx, t, "filter-role-denied", saName, allowedNS, deniedNS, "filter-binding-denied")

	// Create AudiciaSource with deny filter for the denied namespace.
	spec := &audiciav1alpha1.AudiciaSourceSpec{
		Filters: []audiciav1alpha1.Filter{
			{
				Action:           audiciav1alpha1.FilterActionDeny,
				NamespacePattern: "^" + deniedNS + "$",
			},
		},
	}
	createAudiciaSource(ctx, t, "filter-source", allowedNS, spec)

	// Generate audit events as the SA in both namespaces.
	actAsServiceAccount(ctx, t, saName, allowedNS, func(cs *kubernetes.Clientset) {
		if _, err := cs.CoreV1().Pods(allowedNS).List(ctx, metav1.ListOptions{}); err != nil {
			t.Fatalf("SA list pods in allowed ns: %v", err)
		}
		if _, err := cs.CoreV1().Pods(deniedNS).List(ctx, metav1.ListOptions{}); err != nil {
			t.Fatalf("SA list pods in denied ns: %v", err)
		}
	})

	// Wait for report.
	reportName := expectedReportName(saName)
	report := waitForPolicyReport(ctx, t, reportName, allowedNS, defaultTimeout)

	// Assert: rules should only be from the allowed namespace.
	assertRuleExists(t, report.Status.ObservedRules, "", "pods", "list")
	assertNoRuleWithNamespace(t, report.Status.ObservedRules, deniedNS)

	// Verify at least one rule is from the allowed namespace.
	foundAllowed := false
	for _, r := range report.Status.ObservedRules {
		if r.Namespace == allowedNS {
			foundAllowed = true
			break
		}
	}
	if !foundAllowed {
		t.Error("expected at least one rule from the allowed namespace")
	}

	t.Logf("filter test passed: %d rules observed, all from allowed namespace",
		len(report.Status.ObservedRules))
}
