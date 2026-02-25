//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"testing"
	"time"

	authnv1 "k8s.io/api/authentication/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	auditv1 "k8s.io/apiserver/pkg/apis/audit/v1"

	audiciav1alpha1 "github.com/felixnotka/audicia/operator/pkg/apis/audicia.io/v1alpha1"
)

// TestLimits_MaxRulesPerReport verifies that MaxRulesPerReport truncates the
// observed rules list when more distinct rules are generated than the limit.
func TestLimits_MaxRulesPerReport(t *testing.T) {
	ctx := context.Background()
	suffix := uniqueSuffix()
	ns := "e2e-maxrules-" + suffix
	saName := "maxrules-sa"

	createNamespace(ctx, t, ns)
	createServiceAccount(ctx, t, saName, ns)

	// Grant SA broad permissions.
	createRole(ctx, t, "maxrules-role", ns, []rbacv1.PolicyRule{
		{
			APIGroups: []string{"", "apps"},
			Resources: []string{"pods", "services", "configmaps", "secrets", "deployments", "statefulsets"},
			Verbs:     []string{"get", "list"},
		},
	})
	bindRoleToSA(ctx, t, "maxrules-role", saName, ns, ns, "maxrules-binding")

	// Set MaxRulesPerReport to 3 to easily test truncation.
	spec := &audiciav1alpha1.AudiciaSourceSpec{
		SourceType: audiciav1alpha1.SourceTypeWebhook,
		Webhook: &audiciav1alpha1.WebhookConfig{
			Port:                8443,
			TLSSecretName:       "audicia-webhook-tls",
			RateLimitPerSecond:  100,
			MaxRequestBodyBytes: 1048576,
		},
		Limits: audiciav1alpha1.LimitsConfig{
			MaxRulesPerReport: 3,
		},
	}
	createAudiciaSource(ctx, t, "maxrules-source", ns, spec)
	time.Sleep(5 * time.Second)

	httpClient, webhookURL := setupLimitsWebhookClient(t)
	waitForWebhookReady(t, httpClient, webhookURL)

	// Inject 6 distinct rules (different resources).
	saUsername := fmt.Sprintf("system:serviceaccount:%s:%s", ns, saName)
	events := buildAuditEvents(saUsername, ns, []auditAction{
		{resource: "pods", verb: "get"},
		{resource: "services", verb: "get"},
		{resource: "configmaps", verb: "get"},
		{resource: "secrets", verb: "get"},
		{resource: "pods", verb: "list"},
		{resource: "services", verb: "list"},
	})
	postAuditEvents(t, httpClient, webhookURL, events)

	// Wait for report.
	reportName := expectedReportName(saName)
	report := waitForPolicyReportCondition(ctx, t, reportName, ns, func(r *audiciav1alpha1.AudiciaPolicyReport) bool {
		return len(r.Status.ObservedRules) > 0
	}, defaultTimeout)

	// MaxRulesPerReport=3: should have at most 3 observed rules.
	if len(report.Status.ObservedRules) > 3 {
		t.Errorf("expected at most 3 rules (MaxRulesPerReport=3), got %d", len(report.Status.ObservedRules))
	}

	t.Logf("MaxRulesPerReport test passed: %d rules (limit=3)", len(report.Status.ObservedRules))
}

// TestLimits_RetentionDays verifies that rules with old LastSeen timestamps
// are pruned when RetentionDays is set.
func TestLimits_RetentionDays(t *testing.T) {
	ctx := context.Background()
	suffix := uniqueSuffix()
	ns := "e2e-retention-" + suffix
	saName := "retention-sa"

	createNamespace(ctx, t, ns)
	createServiceAccount(ctx, t, saName, ns)

	createRole(ctx, t, "retention-role", ns, []rbacv1.PolicyRule{
		{
			APIGroups: []string{""},
			Resources: []string{"pods", "secrets"},
			Verbs:     []string{"get"},
		},
	})
	bindRoleToSA(ctx, t, "retention-role", saName, ns, ns, "retention-binding")

	// Set RetentionDays to 1: rules with LastSeen > 24 hours ago should be pruned.
	spec := &audiciav1alpha1.AudiciaSourceSpec{
		SourceType: audiciav1alpha1.SourceTypeWebhook,
		Webhook: &audiciav1alpha1.WebhookConfig{
			Port:                8443,
			TLSSecretName:       "audicia-webhook-tls",
			RateLimitPerSecond:  100,
			MaxRequestBodyBytes: 1048576,
		},
		Limits: audiciav1alpha1.LimitsConfig{
			RetentionDays: 1,
		},
	}
	createAudiciaSource(ctx, t, "retention-source", ns, spec)
	time.Sleep(5 * time.Second)

	httpClient, webhookURL := setupLimitsWebhookClient(t)
	waitForWebhookReady(t, httpClient, webhookURL)

	saUsername := fmt.Sprintf("system:serviceaccount:%s:%s", ns, saName)

	// Build events: one recent (now), one old (48 hours ago).
	now := metav1.NewMicroTime(time.Now())
	old := metav1.NewMicroTime(time.Now().Add(-48 * time.Hour))

	eventList := auditv1.EventList{
		TypeMeta: metav1.TypeMeta{APIVersion: "audit.k8s.io/v1", Kind: "EventList"},
		Items: []auditv1.Event{
			{
				TypeMeta:   metav1.TypeMeta{APIVersion: "audit.k8s.io/v1", Kind: "Event"},
				Level:      auditv1.LevelRequestResponse,
				AuditID:    types.UID("recent-" + suffix),
				Stage:      auditv1.StageResponseComplete,
				Verb:       "get",
				RequestURI: fmt.Sprintf("/api/v1/namespaces/%s/pods", ns),
				User: authnv1.UserInfo{
					Username: saUsername,
					Groups:   []string{"system:serviceaccounts", "system:authenticated"},
				},
				ObjectRef: &auditv1.ObjectReference{
					APIVersion: "v1",
					Resource:   "pods",
					Namespace:  ns,
				},
				ResponseStatus:          &metav1.Status{Code: 200},
				RequestReceivedTimestamp: now,
				StageTimestamp:           now,
				SourceIPs:               []string{"10.0.0.1"},
			},
			{
				TypeMeta:   metav1.TypeMeta{APIVersion: "audit.k8s.io/v1", Kind: "Event"},
				Level:      auditv1.LevelRequestResponse,
				AuditID:    types.UID("old-" + suffix),
				Stage:      auditv1.StageResponseComplete,
				Verb:       "get",
				RequestURI: fmt.Sprintf("/api/v1/namespaces/%s/secrets", ns),
				User: authnv1.UserInfo{
					Username: saUsername,
					Groups:   []string{"system:serviceaccounts", "system:authenticated"},
				},
				ObjectRef: &auditv1.ObjectReference{
					APIVersion: "v1",
					Resource:   "secrets",
					Namespace:  ns,
				},
				ResponseStatus:          &metav1.Status{Code: 200},
				RequestReceivedTimestamp: old,
				StageTimestamp:           old,
				SourceIPs:               []string{"10.0.0.1"},
			},
		},
	}

	body, err := json.Marshal(eventList)
	if err != nil {
		t.Fatalf("marshal events: %v", err)
	}
	resp := postAuditEventsRaw(t, httpClient, webhookURL, body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("webhook returned %d, expected 200", resp.StatusCode)
	}

	// Wait for report.
	reportName := expectedReportName(saName)
	report := waitForPolicyReportCondition(ctx, t, reportName, ns, func(r *audiciav1alpha1.AudiciaPolicyReport) bool {
		return r.Status.EventsProcessed > 0
	}, defaultTimeout)

	// RetentionDays=1: the old event (48h ago) should be pruned.
	// Only the recent pods/get rule should survive.
	for _, rule := range report.Status.ObservedRules {
		if containsStr(rule.Resources, "secrets") {
			t.Error("expected 'secrets' rule to be pruned by retention (48h old, retention=1d)")
		}
	}

	assertRuleExists(t, report.Status.ObservedRules, "", "pods", "get")

	t.Logf("RetentionDays test passed: %d rules (old events pruned)",
		len(report.Status.ObservedRules))
}

// setupLimitsWebhookClient creates a TLS-configured HTTP client with port-forward.
func setupLimitsWebhookClient(t *testing.T) (*http.Client, string) {
	t.Helper()

	localPort := "18443"

	pfCtx, pfCancel := context.WithCancel(context.Background())
	pfCmd := exec.CommandContext(pfCtx, "kubectl", "port-forward",
		"-n", helmNamespace,
		"svc/"+helmReleaseName+"-audicia-operator-webhook",
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
