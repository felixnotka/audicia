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

// TestWebhookRateLimiting verifies that the webhook respects RateLimitPerSecond
// and returns 429 when the limit is exceeded.
func TestWebhookRateLimiting(t *testing.T) {
	ctx := context.Background()
	suffix := uniqueSuffix()
	ns := "e2e-ratelimit-" + suffix
	saName := "ratelimit-sa"

	createNamespace(ctx, t, ns)
	createServiceAccount(ctx, t, saName, ns)

	createRole(ctx, t, "ratelimit-role", ns, []rbacv1.PolicyRule{
		{
			APIGroups: []string{""},
			Resources: []string{"pods"},
			Verbs:     []string{"list"},
		},
	})
	bindRoleToSA(ctx, t, "ratelimit-role", saName, ns, ns, "ratelimit-binding")

	// Very low rate limit to reliably trigger 429.
	spec := &audiciav1alpha1.AudiciaSourceSpec{
		SourceType: audiciav1alpha1.SourceTypeWebhook,
		Webhook: &audiciav1alpha1.WebhookConfig{
			Port:                8443,
			TLSSecretName:       "audicia-webhook-tls",
			RateLimitPerSecond:  2,
			MaxRequestBodyBytes: 1048576,
		},
	}
	createAudiciaSource(ctx, t, "ratelimit-source", ns, spec)
	time.Sleep(5 * time.Second)

	httpClient, webhookURL := setupRateLimitWebhookClient(t)
	waitForWebhookReady(t, httpClient, webhookURL)

	saUsername := fmt.Sprintf("system:serviceaccount:%s:%s", ns, saName)

	var count200, count429 int
	for i := 0; i < 20; i++ {
		events := buildAuditEvents(saUsername, ns, []auditAction{
			{resource: "pods", verb: "list"},
		})
		// Override AuditID to be unique per request.
		events.Items[0].AuditID = types.UID(fmt.Sprintf("ratelimit-%s-%d", suffix, i))

		body, err := json.Marshal(events)
		if err != nil {
			t.Fatalf("marshal events: %v", err)
		}

		resp := postAuditEventsRaw(t, httpClient, webhookURL, body)
		switch resp.StatusCode {
		case http.StatusOK:
			count200++
		case http.StatusTooManyRequests:
			count429++
		default:
			t.Logf("unexpected status %d on request %d", resp.StatusCode, i)
		}
	}

	if count429 == 0 {
		t.Error("expected at least one 429 response, got none")
	}
	if count200 == 0 {
		t.Error("expected at least one 200 response, got none")
	}
	t.Logf("rate limit test: 200s=%d, 429s=%d", count200, count429)
}

// TestWebhookDeduplication verifies that duplicate AuditIDs are silently dropped
// and don't inflate EventsProcessed.
func TestWebhookDeduplication(t *testing.T) {
	// Wait for previous webhook server to release the port.
	time.Sleep(3 * time.Second)

	ctx := context.Background()
	suffix := uniqueSuffix()
	ns := "e2e-dedup-" + suffix
	saName := "dedup-sa"

	createNamespace(ctx, t, ns)
	createServiceAccount(ctx, t, saName, ns)

	createRole(ctx, t, "dedup-role", ns, []rbacv1.PolicyRule{
		{
			APIGroups: []string{""},
			Resources: []string{"pods"},
			Verbs:     []string{"list"},
		},
	})
	bindRoleToSA(ctx, t, "dedup-role", saName, ns, ns, "dedup-binding")

	spec := &audiciav1alpha1.AudiciaSourceSpec{
		SourceType: audiciav1alpha1.SourceTypeWebhook,
		Webhook: &audiciav1alpha1.WebhookConfig{
			Port:                8443,
			TLSSecretName:       "audicia-webhook-tls",
			RateLimitPerSecond:  100,
			MaxRequestBodyBytes: 1048576,
		},
	}
	createAudiciaSource(ctx, t, "dedup-source", ns, spec)
	time.Sleep(5 * time.Second)

	httpClient, webhookURL := setupRateLimitWebhookClient(t)
	waitForWebhookReady(t, httpClient, webhookURL)

	saUsername := fmt.Sprintf("system:serviceaccount:%s:%s", ns, saName)
	now := metav1.NewMicroTime(time.Now())
	duplicateAuditID := types.UID("dedup-test-" + suffix)

	// Build 3 events with the same AuditID.
	var items []auditv1.Event
	for i := 0; i < 3; i++ {
		items = append(items, auditv1.Event{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "audit.k8s.io/v1",
				Kind:       "Event",
			},
			Level:      auditv1.LevelRequestResponse,
			AuditID:    duplicateAuditID,
			Stage:      auditv1.StageResponseComplete,
			Verb:       "list",
			RequestURI: fmt.Sprintf("/api/v1/namespaces/%s/pods", ns),
			User: authnv1.UserInfo{
				Username: saUsername,
				Groups:   []string{"system:serviceaccounts", "system:serviceaccounts:" + ns, "system:authenticated"},
			},
			ObjectRef: &auditv1.ObjectReference{
				APIVersion: "v1",
				Resource:   "pods",
				Namespace:  ns,
			},
			ResponseStatus:           &metav1.Status{Code: 200},
			RequestReceivedTimestamp: now,
			StageTimestamp:           now,
			SourceIPs:                []string{"10.0.0.1"},
		})
	}

	eventList := auditv1.EventList{
		TypeMeta: metav1.TypeMeta{APIVersion: "audit.k8s.io/v1", Kind: "EventList"},
		Items:    items,
	}
	postAuditEvents(t, httpClient, webhookURL, eventList)

	// Wait for report.
	reportName := expectedReportName(saName)
	report := waitForPolicyReportCondition(ctx, t, reportName, ns, func(r *audiciav1alpha1.AudiciaPolicyReport) bool {
		return r.Status.EventsProcessed > 0
	}, defaultTimeout)

	// Dedup should reduce 3 duplicate events to 1.
	if report.Status.EventsProcessed != 1 {
		t.Errorf("expected EventsProcessed=1 (dedup), got %d", report.Status.EventsProcessed)
	}

	// POST the same events again (cross-request dedup).
	postAuditEvents(t, httpClient, webhookURL, eventList)
	time.Sleep(10 * time.Second)

	report = getPolicyReport(ctx, t, reportName, ns)
	if report.Status.EventsProcessed != 1 {
		t.Errorf("cross-request dedup: expected EventsProcessed still 1, got %d", report.Status.EventsProcessed)
	}

	t.Logf("dedup test passed: EventsProcessed=%d", report.Status.EventsProcessed)
}

// setupRateLimitWebhookClient is identical to setupWebhookClient but
// scoped for the webhook limits tests (using the same port).
func setupRateLimitWebhookClient(t *testing.T) (*http.Client, string) {
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
