//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"testing"
	"time"
)

// TestMetricsEndpoint verifies that the operator exposes Prometheus metrics
// with expected counters at non-zero values. This test should run after other
// tests have generated events and reports.
func TestMetricsEndpoint(t *testing.T) {
	ctx := context.Background()
	localPort := "18080"

	// Start port-forward to the operator metrics port.
	pfCtx, pfCancel := context.WithCancel(ctx)
	pfCmd := exec.CommandContext(pfCtx, "kubectl", "port-forward",
		"-n", helmNamespace,
		"deploy/"+helmFullName,
		localPort+":8080",
		"--context", "kind-"+kindClusterName)

	if err := pfCmd.Start(); err != nil {
		pfCancel()
		t.Fatalf("start metrics port-forward: %v", err)
	}
	t.Cleanup(func() {
		pfCancel()
		_ = pfCmd.Wait()
	})

	// Wait for port-forward to be ready.
	metricsURL := fmt.Sprintf("http://localhost:%s/metrics", localPort)
	var body string
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(metricsURL)
		if err == nil {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				body = string(b)
				break
			}
		}
		time.Sleep(1 * time.Second)
	}

	if body == "" {
		t.Fatal("failed to fetch /metrics within 30s")
	}

	t.Logf("fetched %d bytes from /metrics", len(body))

	// Assert key metrics exist.
	assertMetricExists(t, body, "audicia_events_processed_total")
	assertMetricExists(t, body, "audicia_reports_updated_total")
	assertMetricExists(t, body, "audicia_rules_generated_total")
	assertMetricExists(t, body, "audicia_pipeline_latency_seconds_count")

	// Assert key counters have non-zero values.
	assertMetricPositive(t, body, "audicia_events_processed_total")
	assertMetricPositive(t, body, "audicia_reports_updated_total")

	t.Log("metrics endpoint test passed")
}
