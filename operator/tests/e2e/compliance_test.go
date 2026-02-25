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

// TestComplianceScoring verifies that a subject with broad RBAC but narrow
// actual usage gets a low compliance score with excess and sensitive excess flags.
func TestComplianceScoring(t *testing.T) {
	ctx := context.Background()
	suffix := uniqueSuffix()
	ns := "e2e-compliance-" + suffix
	saName := "compliance-sa"
	crName := "compliance-broad-" + suffix

	// Setup namespace and SA.
	createNamespace(ctx, t, ns)
	createServiceAccount(ctx, t, saName, ns)

	// Create a broad ClusterRole with many separate rules.
	// Each PolicyRule is counted individually by the RBAC resolver.
	// Secrets are in their own rule so they show up as sensitive excess.
	createClusterRole(ctx, t, crName, []rbacv1.PolicyRule{
		{
			APIGroups: []string{""},
			Resources: []string{"pods"},
			Verbs:     []string{"get", "list", "watch", "create", "update", "delete"},
		},
		{
			APIGroups: []string{""},
			Resources: []string{"configmaps"},
			Verbs:     []string{"get", "list", "watch", "create", "update", "delete"},
		},
		{
			APIGroups: []string{""},
			Resources: []string{"secrets"},
			Verbs:     []string{"get", "list", "watch", "create", "update", "delete"},
		},
		{
			APIGroups: []string{""},
			Resources: []string{"services"},
			Verbs:     []string{"get", "list", "watch", "create", "update", "delete"},
		},
		{
			APIGroups: []string{"apps"},
			Resources: []string{"deployments"},
			Verbs:     []string{"get", "list", "watch", "create", "update", "delete"},
		},
	})

	// Bind via RoleBinding (namespace-scoped).
	bindClusterRoleToSAViaRoleBinding(ctx, t, crName, saName, ns, ns, "compliance-binding")

	// Create AudiciaSource.
	createAudiciaSource(ctx, t, "compliance-source", ns, nil)

	// Act as SA: only list pods (1 of 5 rules exercised).
	actAsServiceAccount(ctx, t, saName, ns, func(cs *kubernetes.Clientset) {
		if _, err := cs.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{}); err != nil {
			t.Fatalf("SA list pods: %v", err)
		}
	})

	// Wait for report with compliance data.
	reportName := expectedReportName(saName)
	report := waitForPolicyReportCondition(ctx, t, reportName, ns, func(r *audiciav1alpha1.AudiciaPolicyReport) bool {
		return r.Status.Compliance != nil
	}, defaultTimeout)

	compliance := report.Status.Compliance

	// Exact score: 1 used / 5 total * 100 = 20.
	if compliance.Score != 20 {
		t.Errorf("expected score=20, got %d", compliance.Score)
	}
	if compliance.Severity != audiciav1alpha1.ComplianceSeverityRed {
		t.Errorf("expected severity=Red, got %s", compliance.Severity)
	}
	if compliance.UsedCount != 1 {
		t.Errorf("expected usedCount=1, got %d", compliance.UsedCount)
	}
	if compliance.ExcessCount != 4 {
		t.Errorf("expected excessCount=4, got %d", compliance.ExcessCount)
	}
	if compliance.UncoveredCount != 0 {
		t.Errorf("expected uncoveredCount=0, got %d", compliance.UncoveredCount)
	}
	if !compliance.HasSensitiveExcess {
		t.Error("expected hasSensitiveExcess=true (secrets rule is excess)")
	}
	if !containsStr(compliance.SensitiveExcess, "secrets") {
		t.Errorf("expected 'secrets' in sensitiveExcess, got %v", compliance.SensitiveExcess)
	}

	t.Logf("compliance test passed: score=%d, severity=%s, used=%d, excess=%d",
		compliance.Score, compliance.Severity, compliance.UsedCount, compliance.ExcessCount)
}
