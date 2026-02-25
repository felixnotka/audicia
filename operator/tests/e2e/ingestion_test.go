//go:build e2e

package e2e

import (
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// TestFileIngestion verifies the full happy path: create an SA with known RBAC,
// perform API calls as that SA, and assert the operator produces a correct
// AudiciaPolicyReport with observed rules and suggested policy.
func TestFileIngestion(t *testing.T) {
	ctx := context.Background()
	suffix := uniqueSuffix()
	ns := "e2e-ingestion-" + suffix
	saName := "ingestion-sa"

	// Setup namespace and SA.
	createNamespace(ctx, t, ns)
	createServiceAccount(ctx, t, saName, ns)

	// Grant SA: get,list pods + create configmaps in its namespace.
	createRole(ctx, t, "ingestion-role", ns, []rbacv1.PolicyRule{
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
	bindRoleToSA(ctx, t, "ingestion-role", saName, ns, ns, "ingestion-binding")

	// Create AudiciaSource.
	createAudiciaSource(ctx, t, "ingestion-source", ns, nil)

	// Generate audit events as the SA.
	actAsServiceAccount(ctx, t, saName, ns, func(cs *kubernetes.Clientset) {
		// List pods.
		if _, err := cs.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{}); err != nil {
			t.Fatalf("SA list pods: %v", err)
		}
		// Get a specific pod (will 404 but still generates an audit event).
		_, _ = cs.CoreV1().Pods(ns).Get(ctx, "nonexistent", metav1.GetOptions{})

		// Create a configmap.
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "e2e-test-cm-" + suffix, Namespace: ns},
			Data:       map[string]string{"key": "value"},
		}
		if _, err := cs.CoreV1().ConfigMaps(ns).Create(ctx, cm, metav1.CreateOptions{}); err != nil {
			t.Fatalf("SA create configmap: %v", err)
		}
	})

	// Wait for the policy report.
	reportName := expectedReportName(saName)
	report := waitForPolicyReport(ctx, t, reportName, ns, defaultTimeout)

	// Assert subject is correct.
	if report.Spec.Subject.Name != saName {
		t.Errorf("expected subject name %q, got %q", saName, report.Spec.Subject.Name)
	}
	if report.Spec.Subject.Namespace != ns {
		t.Errorf("expected subject namespace %q, got %q", ns, report.Spec.Subject.Namespace)
	}
	if report.Spec.Subject.Kind != "ServiceAccount" {
		t.Errorf("expected subject kind ServiceAccount, got %q", report.Spec.Subject.Kind)
	}

	// Assert observed rules contain expected entries.
	assertRuleExists(t, report.Status.ObservedRules, "", "pods", "list")
	assertRuleExists(t, report.Status.ObservedRules, "", "pods", "get")
	assertRuleExists(t, report.Status.ObservedRules, "", "configmaps", "create")

	// Assert suggested policy has manifests.
	if report.Status.SuggestedPolicy == nil || len(report.Status.SuggestedPolicy.Manifests) == 0 {
		t.Error("expected suggestedPolicy with manifests, got nil or empty")
	}

	// Assert events were processed.
	if report.Status.EventsProcessed < 3 {
		t.Errorf("expected EventsProcessed >= 3, got %d", report.Status.EventsProcessed)
	}

	// Check AudiciaPolicyReport has Ready=True/PolicyGenerated condition.
	readyCond := meta.FindStatusCondition(report.Status.Conditions, "Ready")
	if readyCond == nil || readyCond.Status != metav1.ConditionTrue {
		t.Error("expected Ready=True condition on report")
	}
	if readyCond != nil && readyCond.Reason != "PolicyGenerated" {
		t.Errorf("expected Ready reason=PolicyGenerated, got %q", readyCond.Reason)
	}

	// SuggestedPolicy manifests should contain Role + RoleBinding YAML.
	if report.Status.SuggestedPolicy != nil {
		manifests := strings.Join(report.Status.SuggestedPolicy.Manifests, "\n")
		if !strings.Contains(manifests, "kind: Role") {
			t.Error("expected 'kind: Role' in suggested manifests")
		}
		if !strings.Contains(manifests, "kind: RoleBinding") {
			t.Error("expected 'kind: RoleBinding' in suggested manifests")
		}
	}

	t.Logf("ingestion test passed: %d rules observed, %d events processed",
		len(report.Status.ObservedRules), report.Status.EventsProcessed)
}
