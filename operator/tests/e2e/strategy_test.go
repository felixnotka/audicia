//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os/exec"
	"testing"
	"time"

	rbacv1 "k8s.io/api/rbac/v1"

	audiciav1alpha1 "github.com/felixnotka/audicia/operator/pkg/apis/audicia.io/v1alpha1"
)

// TestStrategyKnobs verifies that PolicyStrategy knobs (VerbMerge, Wildcards)
// produce the expected output in SuggestedPolicy manifests.
func TestStrategyKnobs(t *testing.T) {
	t.Run("VerbMerge/Smart", testVerbMergeSmart)
	t.Run("VerbMerge/Exact", testVerbMergeExact)
	t.Run("Wildcards/Safe", testWildcardsSafe)
	t.Run("Wildcards/Forbidden", testWildcardsForbidden)
}

func testVerbMergeSmart(t *testing.T) {
	ctx := context.Background()
	suffix := uniqueSuffix()
	ns := "e2e-strategy-smart-" + suffix
	saName := "strategy-smart-sa"

	createNamespace(ctx, t, ns)
	createServiceAccount(ctx, t, saName, ns)

	createRole(ctx, t, "strategy-smart-role", ns, []rbacv1.PolicyRule{
		{
			APIGroups: []string{""},
			Resources: []string{"pods"},
			Verbs:     []string{"get", "list", "watch"},
		},
	})
	bindRoleToSA(ctx, t, "strategy-smart-role", saName, ns, ns, "strategy-smart-binding")

	spec := &audiciav1alpha1.AudiciaSourceSpec{
		SourceType: audiciav1alpha1.SourceTypeWebhook,
		Webhook: &audiciav1alpha1.WebhookConfig{
			Port:                8443,
			TLSSecretName:       "audicia-webhook-tls",
			RateLimitPerSecond:  100,
			MaxRequestBodyBytes: 1048576,
		},
		PolicyStrategy: audiciav1alpha1.PolicyStrategy{
			VerbMerge: audiciav1alpha1.VerbMergeSmart,
		},
	}
	createAudiciaSource(ctx, t, "strategy-smart-source", ns, spec)
	time.Sleep(5 * time.Second)

	httpClient, webhookURL := setupWebhookClient(t)
	waitForWebhookReady(t, httpClient, webhookURL)

	saUsername := fmt.Sprintf("system:serviceaccount:%s:%s", ns, saName)
	events := buildAuditEvents(saUsername, ns, []auditAction{
		{resource: "pods", verb: "get"},
		{resource: "pods", verb: "list"},
		{resource: "pods", verb: "watch"},
	})
	postAuditEvents(t, httpClient, webhookURL, events)

	reportName := expectedReportName(saName)
	report := waitForPolicyReportCondition(ctx, t, reportName, ns, func(r *audiciav1alpha1.AudiciaPolicyReport) bool {
		return r.Status.SuggestedPolicy != nil && len(r.Status.SuggestedPolicy.Manifests) > 0
	}, defaultTimeout)

	role := parseRoleFromManifests(t, report.Status.SuggestedPolicy.Manifests)

	// Smart merge: get+list+watch should produce 1 rule with 3 verbs.
	if len(role.Rules) != 1 {
		t.Errorf("VerbMerge=Smart: expected 1 rule, got %d", len(role.Rules))
	}
	if len(role.Rules) > 0 && len(role.Rules[0].Verbs) != 3 {
		t.Errorf("VerbMerge=Smart: expected 3 verbs, got %d: %v", len(role.Rules[0].Verbs), role.Rules[0].Verbs)
	}

	t.Logf("VerbMerge/Smart passed: %d rules, verbs=%v", len(role.Rules), role.Rules[0].Verbs)
}

func testVerbMergeExact(t *testing.T) {
	// Small delay to let previous webhook server release the port.
	time.Sleep(3 * time.Second)

	ctx := context.Background()
	suffix := uniqueSuffix()
	ns := "e2e-strategy-exact-" + suffix
	saName := "strategy-exact-sa"

	createNamespace(ctx, t, ns)
	createServiceAccount(ctx, t, saName, ns)

	createRole(ctx, t, "strategy-exact-role", ns, []rbacv1.PolicyRule{
		{
			APIGroups: []string{""},
			Resources: []string{"pods"},
			Verbs:     []string{"get", "list"},
		},
	})
	bindRoleToSA(ctx, t, "strategy-exact-role", saName, ns, ns, "strategy-exact-binding")

	spec := &audiciav1alpha1.AudiciaSourceSpec{
		SourceType: audiciav1alpha1.SourceTypeWebhook,
		Webhook: &audiciav1alpha1.WebhookConfig{
			Port:                8443,
			TLSSecretName:       "audicia-webhook-tls",
			RateLimitPerSecond:  100,
			MaxRequestBodyBytes: 1048576,
		},
		PolicyStrategy: audiciav1alpha1.PolicyStrategy{
			VerbMerge: audiciav1alpha1.VerbMergeExact,
		},
	}
	createAudiciaSource(ctx, t, "strategy-exact-source", ns, spec)
	time.Sleep(5 * time.Second)

	httpClient, webhookURL := setupWebhookClient(t)
	waitForWebhookReady(t, httpClient, webhookURL)

	saUsername := fmt.Sprintf("system:serviceaccount:%s:%s", ns, saName)
	events := buildAuditEvents(saUsername, ns, []auditAction{
		{resource: "pods", verb: "get"},
		{resource: "pods", verb: "list"},
	})
	postAuditEvents(t, httpClient, webhookURL, events)

	reportName := expectedReportName(saName)
	report := waitForPolicyReportCondition(ctx, t, reportName, ns, func(r *audiciav1alpha1.AudiciaPolicyReport) bool {
		return r.Status.SuggestedPolicy != nil && len(r.Status.SuggestedPolicy.Manifests) > 0
	}, defaultTimeout)

	role := parseRoleFromManifests(t, report.Status.SuggestedPolicy.Manifests)

	// Exact: each verb should produce a separate rule.
	if len(role.Rules) != 2 {
		t.Errorf("VerbMerge=Exact: expected 2 rules, got %d", len(role.Rules))
	}

	t.Logf("VerbMerge/Exact passed: %d rules", len(role.Rules))
}

func testWildcardsSafe(t *testing.T) {
	time.Sleep(3 * time.Second)

	ctx := context.Background()
	suffix := uniqueSuffix()
	ns := "e2e-strategy-wcsafe-" + suffix
	saName := "strategy-wildcard-sa"

	createNamespace(ctx, t, ns)
	createServiceAccount(ctx, t, saName, ns)

	createRole(ctx, t, "strategy-wc-role", ns, []rbacv1.PolicyRule{
		{
			APIGroups: []string{""},
			Resources: []string{"pods"},
			Verbs:     []string{"get", "list", "watch", "create", "update", "patch", "delete", "deletecollection"},
		},
	})
	bindRoleToSA(ctx, t, "strategy-wc-role", saName, ns, ns, "strategy-wc-binding")

	spec := &audiciav1alpha1.AudiciaSourceSpec{
		SourceType: audiciav1alpha1.SourceTypeWebhook,
		Webhook: &audiciav1alpha1.WebhookConfig{
			Port:                8443,
			TLSSecretName:       "audicia-webhook-tls",
			RateLimitPerSecond:  100,
			MaxRequestBodyBytes: 1048576,
		},
		PolicyStrategy: audiciav1alpha1.PolicyStrategy{
			VerbMerge: audiciav1alpha1.VerbMergeSmart,
			Wildcards: audiciav1alpha1.WildcardModeSafe,
		},
	}
	createAudiciaSource(ctx, t, "strategy-wcsafe-source", ns, spec)
	time.Sleep(5 * time.Second)

	httpClient, webhookURL := setupWebhookClient(t)
	waitForWebhookReady(t, httpClient, webhookURL)

	saUsername := fmt.Sprintf("system:serviceaccount:%s:%s", ns, saName)
	events := buildAuditEvents(saUsername, ns, []auditAction{
		{resource: "pods", verb: "get"},
		{resource: "pods", verb: "list"},
		{resource: "pods", verb: "watch"},
		{resource: "pods", verb: "create"},
		{resource: "pods", verb: "update"},
		{resource: "pods", verb: "patch"},
		{resource: "pods", verb: "delete"},
		{resource: "pods", verb: "deletecollection"},
	})
	postAuditEvents(t, httpClient, webhookURL, events)

	reportName := expectedReportName(saName)
	report := waitForPolicyReportCondition(ctx, t, reportName, ns, func(r *audiciav1alpha1.AudiciaPolicyReport) bool {
		return r.Status.SuggestedPolicy != nil && len(r.Status.SuggestedPolicy.Manifests) > 0
	}, defaultTimeout)

	role := parseRoleFromManifests(t, report.Status.SuggestedPolicy.Manifests)

	// Wildcards=Safe with all 8 verbs should collapse to ["*"].
	if len(role.Rules) != 1 {
		t.Errorf("Wildcards=Safe: expected 1 rule, got %d", len(role.Rules))
	}
	if len(role.Rules) > 0 && !containsStr(role.Rules[0].Verbs, "*") {
		t.Errorf("Wildcards=Safe: expected verb '*', got %v", role.Rules[0].Verbs)
	}

	t.Logf("Wildcards/Safe passed: %d rules, verbs=%v", len(role.Rules), role.Rules[0].Verbs)
}

func testWildcardsForbidden(t *testing.T) {
	time.Sleep(3 * time.Second)

	ctx := context.Background()
	suffix := uniqueSuffix()
	ns := "e2e-strategy-wcforbid-" + suffix
	saName := "strategy-wcforbid-sa"

	createNamespace(ctx, t, ns)
	createServiceAccount(ctx, t, saName, ns)

	createRole(ctx, t, "strategy-wcf-role", ns, []rbacv1.PolicyRule{
		{
			APIGroups: []string{""},
			Resources: []string{"pods"},
			Verbs:     []string{"get", "list", "watch", "create", "update", "patch", "delete", "deletecollection"},
		},
	})
	bindRoleToSA(ctx, t, "strategy-wcf-role", saName, ns, ns, "strategy-wcf-binding")

	spec := &audiciav1alpha1.AudiciaSourceSpec{
		SourceType: audiciav1alpha1.SourceTypeWebhook,
		Webhook: &audiciav1alpha1.WebhookConfig{
			Port:                8443,
			TLSSecretName:       "audicia-webhook-tls",
			RateLimitPerSecond:  100,
			MaxRequestBodyBytes: 1048576,
		},
		PolicyStrategy: audiciav1alpha1.PolicyStrategy{
			VerbMerge: audiciav1alpha1.VerbMergeSmart,
			Wildcards: audiciav1alpha1.WildcardModeForbidden,
		},
	}
	createAudiciaSource(ctx, t, "strategy-wcforbid-source", ns, spec)
	time.Sleep(5 * time.Second)

	httpClient, webhookURL := setupWebhookClient(t)
	waitForWebhookReady(t, httpClient, webhookURL)

	saUsername := fmt.Sprintf("system:serviceaccount:%s:%s", ns, saName)
	events := buildAuditEvents(saUsername, ns, []auditAction{
		{resource: "pods", verb: "get"},
		{resource: "pods", verb: "list"},
		{resource: "pods", verb: "watch"},
		{resource: "pods", verb: "create"},
		{resource: "pods", verb: "update"},
		{resource: "pods", verb: "patch"},
		{resource: "pods", verb: "delete"},
		{resource: "pods", verb: "deletecollection"},
	})
	postAuditEvents(t, httpClient, webhookURL, events)

	reportName := expectedReportName(saName)
	report := waitForPolicyReportCondition(ctx, t, reportName, ns, func(r *audiciav1alpha1.AudiciaPolicyReport) bool {
		return r.Status.SuggestedPolicy != nil && len(r.Status.SuggestedPolicy.Manifests) > 0
	}, defaultTimeout)

	role := parseRoleFromManifests(t, report.Status.SuggestedPolicy.Manifests)

	// Wildcards=Forbidden: should have individual verbs, no "*".
	if len(role.Rules) < 1 {
		t.Fatal("Wildcards=Forbidden: expected at least 1 rule")
	}

	// With VerbMerge=Smart, all 8 verbs should be merged into 1 rule.
	if len(role.Rules) != 1 {
		t.Errorf("Wildcards=Forbidden: expected 1 rule (Smart merge), got %d", len(role.Rules))
	}

	if len(role.Rules) > 0 {
		verbs := role.Rules[0].Verbs
		if containsStr(verbs, "*") {
			t.Errorf("Wildcards=Forbidden: unexpected wildcard '*' in verbs %v", verbs)
		}
		if len(verbs) != 8 {
			t.Errorf("Wildcards=Forbidden: expected 8 verbs, got %d: %v", len(verbs), verbs)
		}
	}

	t.Logf("Wildcards/Forbidden passed: %d rules, verbs=%v", len(role.Rules), role.Rules[0].Verbs)
}

// setupWebhookClient creates a TLS-configured HTTP client and returns it with
// the webhook URL. Shared by strategy subtests that use the webhook.
func setupWebhookClient(t *testing.T) (*http.Client, string) {
	t.Helper()

	localPort := "18443"

	// Start port-forward.
	pfCtx, pfCancel := context.WithCancel(context.Background())
	pfCmd := exec.CommandContext(pfCtx, "kubectl", "port-forward",
		"-n", helmNamespace,
		"svc/"+helmFullName+"-webhook",
		localPort+":8443",
		"--context", "kind-"+kindClusterName)

	var pfStderr bytes.Buffer
	pfCmd.Stderr = &pfStderr
	if err := pfCmd.Start(); err != nil {
		pfCancel()
		t.Fatalf("start port-forward: %v", err)
	}
	t.Cleanup(func() {
		pfCancel()
		_ = pfCmd.Wait()
	})

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(webhookCACert) {
		t.Fatal("failed to add webhook CA cert to pool")
	}

	httpClient := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs: caPool,
			},
		},
	}

	webhookURL := fmt.Sprintf("https://localhost:%s/", localPort)
	return httpClient, webhookURL
}
