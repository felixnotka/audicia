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
	"strings"
	"testing"
	"time"

	audiciav1alpha1 "github.com/felixnotka/audicia/operator/pkg/apis/audicia.io/v1alpha1"
)

// TestScopeMode verifies that PolicyStrategy.ScopeMode controls the kind of
// RBAC manifests generated for non-SA subjects (users/groups).
// - ClusterScopeAllowed → ClusterRole + ClusterRoleBinding
// - NamespaceStrict     → Role + RoleBinding (when all rules are namespaced)
func TestScopeMode(t *testing.T) {
	t.Run("ClusterScopeAllowed", testScopeModeClusterAllowed)
	t.Run("NamespaceStrict", testScopeModeNamespaceStrict)
}

func testScopeModeClusterAllowed(t *testing.T) {
	ctx := context.Background()
	suffix := uniqueSuffix()
	ns := "e2e-scope-clust-" + suffix
	userName := "scope-clust-user-" + suffix

	createNamespace(ctx, t, ns)

	spec := &audiciav1alpha1.AudiciaSourceSpec{
		SourceType: audiciav1alpha1.SourceTypeWebhook,
		Webhook: &audiciav1alpha1.WebhookConfig{
			Port:                8443,
			TLSSecretName:       "audicia-webhook-tls",
			RateLimitPerSecond:  100,
			MaxRequestBodyBytes: 1048576,
		},
		PolicyStrategy: audiciav1alpha1.PolicyStrategy{
			ScopeMode: audiciav1alpha1.ScopeModeClusterScopeAllowed,
		},
	}
	createAudiciaSource(ctx, t, "scope-clust-source", ns, spec)
	time.Sleep(5 * time.Second)

	httpClient, webhookURL := setupScopeWebhookClient(t)
	waitForWebhookReady(t, httpClient, webhookURL)

	// Inject events attributed to a regular user (not a ServiceAccount).
	// The strategy engine uses ScopeMode only for non-SA subjects.
	events := buildAuditEvents(userName, ns, []auditAction{
		{resource: "pods", verb: "get"},
		{resource: "pods", verb: "list"},
	})
	postAuditEvents(t, httpClient, webhookURL, events)

	reportName := expectedReportName(userName)
	report := waitForPolicyReportCondition(ctx, t, reportName, ns, func(r *audiciav1alpha1.AudiciaPolicyReport) bool {
		return r.Status.SuggestedPolicy != nil && len(r.Status.SuggestedPolicy.Manifests) > 0
	}, defaultTimeout)

	manifests := strings.Join(report.Status.SuggestedPolicy.Manifests, "\n---\n")

	// ClusterScopeAllowed + User: should produce ClusterRole + ClusterRoleBinding.
	if !strings.Contains(manifests, "kind: ClusterRole\n") {
		t.Error("expected ClusterRole in ClusterScopeAllowed mode for user")
	}
	if !strings.Contains(manifests, "kind: ClusterRoleBinding\n") {
		t.Error("expected ClusterRoleBinding in ClusterScopeAllowed mode for user")
	}

	t.Logf("ScopeMode/ClusterScopeAllowed passed: manifests contain ClusterRole")
}

func testScopeModeNamespaceStrict(t *testing.T) {
	time.Sleep(3 * time.Second) // Let previous webhook server release the port.

	ctx := context.Background()
	suffix := uniqueSuffix()
	ns := "e2e-scope-strict-" + suffix
	userName := "scope-strict-user-" + suffix

	createNamespace(ctx, t, ns)

	spec := &audiciav1alpha1.AudiciaSourceSpec{
		SourceType: audiciav1alpha1.SourceTypeWebhook,
		Webhook: &audiciav1alpha1.WebhookConfig{
			Port:                8443,
			TLSSecretName:       "audicia-webhook-tls",
			RateLimitPerSecond:  100,
			MaxRequestBodyBytes: 1048576,
		},
		PolicyStrategy: audiciav1alpha1.PolicyStrategy{
			ScopeMode: audiciav1alpha1.ScopeModeNamespaceStrict,
		},
	}
	createAudiciaSource(ctx, t, "scope-strict-source", ns, spec)
	time.Sleep(5 * time.Second)

	httpClient, webhookURL := setupScopeWebhookClient(t)
	waitForWebhookReady(t, httpClient, webhookURL)

	events := buildAuditEvents(userName, ns, []auditAction{
		{resource: "pods", verb: "get"},
		{resource: "configmaps", verb: "list"},
	})
	postAuditEvents(t, httpClient, webhookURL, events)

	reportName := expectedReportName(userName)
	report := waitForPolicyReportCondition(ctx, t, reportName, ns, func(r *audiciav1alpha1.AudiciaPolicyReport) bool {
		return r.Status.SuggestedPolicy != nil && len(r.Status.SuggestedPolicy.Manifests) > 0
	}, defaultTimeout)

	manifests := strings.Join(report.Status.SuggestedPolicy.Manifests, "\n---\n")

	// NamespaceStrict + User with namespaced resources: should produce Role + RoleBinding.
	if !strings.Contains(manifests, "kind: Role\n") {
		t.Error("expected Role in NamespaceStrict mode for user")
	}
	if !strings.Contains(manifests, "kind: RoleBinding\n") {
		t.Error("expected RoleBinding in NamespaceStrict mode for user")
	}
	// Should NOT contain ClusterRole (all rules are namespaced).
	if strings.Contains(manifests, "kind: ClusterRole\n") {
		t.Error("unexpected ClusterRole in NamespaceStrict mode with namespaced rules")
	}

	t.Logf("ScopeMode/NamespaceStrict passed: manifests contain Role (no ClusterRole)")
}

// setupScopeWebhookClient creates a TLS-configured HTTP client with port-forward.
func setupScopeWebhookClient(t *testing.T) (*http.Client, string) {
	t.Helper()

	localPort := "18443"

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
