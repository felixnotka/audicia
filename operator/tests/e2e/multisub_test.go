//go:build e2e

package e2e

import (
	"context"
	"testing"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// TestMultiSubject verifies that a single AudiciaSource correctly separates
// audit events from two different ServiceAccounts into two distinct
// AudiciaPolicyReports.
func TestMultiSubject(t *testing.T) {
	ctx := context.Background()
	suffix := uniqueSuffix()
	ns := "e2e-multisub-" + suffix
	saAlpha := "multi-sa-alpha"
	saBeta := "multi-sa-beta"

	// Setup namespace and both SAs.
	createNamespace(ctx, t, ns)
	createServiceAccount(ctx, t, saAlpha, ns)
	createServiceAccount(ctx, t, saBeta, ns)

	// Alpha: pods/get,list + configmaps/create
	createRole(ctx, t, "alpha-role", ns, []rbacv1.PolicyRule{
		{
			APIGroups: []string{""},
			Resources: []string{"pods"},
			Verbs:     []string{"get", "list"},
		},
		{
			APIGroups: []string{""},
			Resources: []string{"configmaps"},
			Verbs:     []string{"create"},
		},
	})
	bindRoleToSA(ctx, t, "alpha-role", saAlpha, ns, ns, "alpha-binding")

	// Beta: services/get,list + secrets/create
	createRole(ctx, t, "beta-role", ns, []rbacv1.PolicyRule{
		{
			APIGroups: []string{""},
			Resources: []string{"services"},
			Verbs:     []string{"get", "list"},
		},
		{
			APIGroups: []string{""},
			Resources: []string{"secrets"},
			Verbs:     []string{"create"},
		},
	})
	bindRoleToSA(ctx, t, "beta-role", saBeta, ns, ns, "beta-binding")

	// Create single AudiciaSource.
	createAudiciaSource(ctx, t, "multisub-source", ns, nil)

	// Generate events as alpha.
	actAsServiceAccount(ctx, t, saAlpha, ns, func(cs *kubernetes.Clientset) {
		if _, err := cs.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{}); err != nil {
			t.Fatalf("alpha list pods: %v", err)
		}
		_, _ = cs.CoreV1().Pods(ns).Get(ctx, "nonexistent", metav1.GetOptions{})
	})

	// Generate events as beta.
	actAsServiceAccount(ctx, t, saBeta, ns, func(cs *kubernetes.Clientset) {
		if _, err := cs.CoreV1().Services(ns).List(ctx, metav1.ListOptions{}); err != nil {
			t.Fatalf("beta list services: %v", err)
		}
		_, _ = cs.CoreV1().Services(ns).Get(ctx, "nonexistent", metav1.GetOptions{})
	})

	// Wait for both reports.
	alphaReportName := expectedReportName(saAlpha)
	betaReportName := expectedReportName(saBeta)

	alphaReport := waitForPolicyReport(ctx, t, alphaReportName, ns, defaultTimeout)
	betaReport := waitForPolicyReport(ctx, t, betaReportName, ns, defaultTimeout)

	// Alpha should have pods rules but not services.
	assertRuleExists(t, alphaReport.Status.ObservedRules, "", "pods", "list")
	assertRuleExists(t, alphaReport.Status.ObservedRules, "", "pods", "get")
	for _, r := range alphaReport.Status.ObservedRules {
		if containsStr(r.Resources, "services") {
			t.Error("alpha report should not contain services rules")
		}
	}

	// Beta should have services rules but not pods.
	assertRuleExists(t, betaReport.Status.ObservedRules, "", "services", "list")
	assertRuleExists(t, betaReport.Status.ObservedRules, "", "services", "get")
	for _, r := range betaReport.Status.ObservedRules {
		if containsStr(r.Resources, "pods") {
			t.Error("beta report should not contain pods rules")
		}
	}

	// Verify subjects are distinct.
	if alphaReport.Spec.Subject.Name != saAlpha {
		t.Errorf("alpha report subject: expected %q, got %q", saAlpha, alphaReport.Spec.Subject.Name)
	}
	if betaReport.Spec.Subject.Name != saBeta {
		t.Errorf("beta report subject: expected %q, got %q", saBeta, betaReport.Spec.Subject.Name)
	}

	t.Logf("multisub test passed: alpha has %d rules, beta has %d rules",
		len(alphaReport.Status.ObservedRules), len(betaReport.Status.ObservedRules))
}
