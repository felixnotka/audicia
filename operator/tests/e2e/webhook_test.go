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

// TestWebhookIngestion verifies the webhook ingestion pipeline end-to-end:
// TLS setup, HTTPS POST of crafted audit events, and report generation.
func TestWebhookIngestion(t *testing.T) {
	ctx := context.Background()
	suffix := uniqueSuffix()
	ns := "e2e-webhook-" + suffix
	saName := "webhook-sa"

	// Setup namespace, SA, and RBAC.
	createNamespace(ctx, t, ns)
	createServiceAccount(ctx, t, saName, ns)

	createRole(ctx, t, "webhook-role", ns, []rbacv1.PolicyRule{
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
	bindRoleToSA(ctx, t, "webhook-role", saName, ns, ns, "webhook-binding")

	// Create AudiciaSource for webhook ingestion.
	spec := &audiciav1alpha1.AudiciaSourceSpec{
		SourceType: audiciav1alpha1.SourceTypeWebhook,
		Webhook: &audiciav1alpha1.WebhookConfig{
			Port:                8443,
			TLSSecretName:       "audicia-webhook-tls",
			RateLimitPerSecond:  100,
			MaxRequestBodyBytes: 1048576,
		},
	}
	createAudiciaSource(ctx, t, "webhook-source", ns, spec)

	// Give the operator time to reconcile the source and start the webhook server.
	time.Sleep(5 * time.Second)

	// Start port-forward to the webhook service.
	pfCtx, pfCancel := context.WithCancel(ctx)
	localPort := "18443" // use a non-privileged local port
	pfCmd := exec.CommandContext(pfCtx, "kubectl", "port-forward",
		"-n", helmNamespace,
		"svc/"+helmReleaseName+"-audicia-operator-webhook",
		localPort+":8443",
		"--context", "kind-"+kindClusterName)

	var pfStderr bytes.Buffer
	pfCmd.Stderr = &pfStderr
	if err := pfCmd.Start(); err != nil {
		t.Fatalf("start port-forward: %v", err)
	}
	t.Cleanup(func() {
		pfCancel()
		_ = pfCmd.Wait()
	})

	// Wait for port-forward to be ready.
	webhookURL := fmt.Sprintf("https://localhost:%s/", localPort)
	httpClient := buildWebhookHTTPClient(t)

	waitForWebhookReady(t, httpClient, webhookURL)

	// Craft audit events.
	saUsername := fmt.Sprintf("system:serviceaccount:%s:%s", ns, saName)
	events := buildAuditEvents(saUsername, ns, []auditAction{
		{resource: "pods", verb: "list"},
		{resource: "pods", verb: "get"},
		{resource: "configmaps", verb: "create"},
	})

	// POST events to webhook.
	postAuditEvents(t, httpClient, webhookURL, events)

	// Wait for the policy report to appear.
	reportName := expectedReportName(saName)
	report := waitForPolicyReport(ctx, t, reportName, ns, defaultTimeout)

	// Assertions.
	t.Logf("webhook report: %d observed rules, %d events processed",
		len(report.Status.ObservedRules), report.Status.EventsProcessed)

	assertRuleExists(t, report.Status.ObservedRules, "", "pods", "list")
	assertRuleExists(t, report.Status.ObservedRules, "", "pods", "get")
	assertRuleExists(t, report.Status.ObservedRules, "", "configmaps", "create")

	if report.Status.EventsProcessed < 3 {
		t.Errorf("expected >= 3 events processed, got %d", report.Status.EventsProcessed)
	}

	if report.Status.SuggestedPolicy == nil {
		t.Error("expected non-nil suggestedPolicy")
	}

	t.Logf("webhook test passed: %d rules, %d events",
		len(report.Status.ObservedRules), report.Status.EventsProcessed)
}

type auditAction struct {
	resource string
	verb     string
}

// buildWebhookHTTPClient creates an HTTPS client that trusts the self-signed CA.
func buildWebhookHTTPClient(t *testing.T) *http.Client {
	t.Helper()

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(webhookCACert) {
		t.Fatal("failed to add webhook CA cert to pool")
	}

	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs: caPool,
			},
		},
	}
}

// waitForWebhookReady polls the webhook endpoint until it responds.
func waitForWebhookReady(t *testing.T, client *http.Client, url string) {
	t.Helper()

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		// Send a GET (webhook only accepts POST, so we expect 405 — that's fine).
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			t.Logf("webhook ready (status %d)", resp.StatusCode)
			return
		}
		time.Sleep(1 * time.Second)
	}
	t.Fatal("webhook endpoint not ready after 30s")
}

// buildAuditEvents constructs an auditv1.EventList with the given actions.
func buildAuditEvents(username, namespace string, actions []auditAction) auditv1.EventList {
	now := metav1.NewMicroTime(time.Now())
	var items []auditv1.Event
	for i, a := range actions {
		items = append(items, auditv1.Event{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "audit.k8s.io/v1",
				Kind:       "Event",
			},
			Level:      auditv1.LevelRequestResponse,
			AuditID:    types.UID(fmt.Sprintf("e2e-webhook-%s-%d-%s", namespace, i, uniqueSuffix())),
			Stage:      auditv1.StageResponseComplete,
			Verb:       a.verb,
			RequestURI: fmt.Sprintf("/api/v1/namespaces/%s/%s", namespace, a.resource),
			User: authnv1.UserInfo{
				Username: username,
				Groups: []string{
					"system:serviceaccounts",
					"system:serviceaccounts:" + namespace,
					"system:authenticated",
				},
			},
			ObjectRef: &auditv1.ObjectReference{
				APIVersion: "v1",
				Resource:   a.resource,
				Namespace:  namespace,
			},
			ResponseStatus: &metav1.Status{
				Code: 200,
			},
			RequestReceivedTimestamp: now,
			StageTimestamp:           now,
			SourceIPs:               []string{"10.0.0.1"},
		})
	}

	return auditv1.EventList{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "audit.k8s.io/v1",
			Kind:       "EventList",
		},
		Items: items,
	}
}

// postAuditEvents POSTs an EventList to the webhook endpoint.
func postAuditEvents(t *testing.T, client *http.Client, url string, events auditv1.EventList) {
	t.Helper()

	body, err := json.Marshal(events)
	if err != nil {
		t.Fatalf("marshal events: %v", err)
	}

	resp, err := client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST to webhook: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("webhook returned status %d, expected 200", resp.StatusCode)
	}
	t.Logf("posted %d audit events to webhook", len(events.Items))
}
