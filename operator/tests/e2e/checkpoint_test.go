//go:build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	audiciav1alpha1 "github.com/felixnotka/audicia/operator/pkg/apis/audicia.io/v1alpha1"
)

// TestCheckpointResume verifies that the operator checkpoints its file offset,
// and after a pod restart it resumes from the checkpoint rather than replaying
// the entire file.
func TestCheckpointResume(t *testing.T) {
	ctx := context.Background()
	suffix := uniqueSuffix()
	ns := "e2e-checkpoint-" + suffix
	saName := "checkpoint-sa"
	sourceName := "checkpoint-source"

	// Setup namespace and SA.
	createNamespace(ctx, t, ns)
	createServiceAccount(ctx, t, saName, ns)

	createRole(ctx, t, "checkpoint-role", ns, []rbacv1.PolicyRule{
		{
			APIGroups: []string{""},
			Resources: []string{"pods"},
			Verbs:     []string{"list"},
		},
	})
	bindRoleToSA(ctx, t, "checkpoint-role", saName, ns, ns, "checkpoint-binding")

	// Create AudiciaSource with 5s checkpoint interval.
	spec := &audiciav1alpha1.AudiciaSourceSpec{
		Checkpoint: audiciav1alpha1.CheckpointConfig{
			IntervalSeconds: 5,
			BatchSize:       100,
		},
	}
	createAudiciaSource(ctx, t, sourceName, ns, spec)

	// Phase 1: Generate events and wait for initial report.
	actAsServiceAccount(ctx, t, saName, ns, func(cs *kubernetes.Clientset) {
		for i := 0; i < 5; i++ {
			if _, err := cs.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{}); err != nil {
				t.Fatalf("SA list pods (phase 1, iter %d): %v", i, err)
			}
		}
	})

	reportName := expectedReportName(saName)
	report := waitForPolicyReport(ctx, t, reportName, ns, defaultTimeout)
	t.Logf("phase 1: %d events processed", report.Status.EventsProcessed)

	// Wait for checkpoint to flush (interval is 5s, wait a couple of cycles).
	time.Sleep(12 * time.Second)

	// Verify the source has a non-zero FileOffset and record it.
	src := waitForSource(ctx, t, sourceName, ns, func(s *audiciav1alpha1.AudiciaSource) bool {
		return s.Status.FileOffset > 0
	}, 30*time.Second)
	checkpointOffset := src.Status.FileOffset
	t.Logf("checkpoint fileOffset: %d", checkpointOffset)

	// Kill the operator pod.
	deleteOperatorPod(ctx, t)
	t.Log("operator pod restarted")

	// Phase 2: Generate more events after restart.
	actAsServiceAccount(ctx, t, saName, ns, func(cs *kubernetes.Clientset) {
		for i := 0; i < 3; i++ {
			if _, err := cs.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{}); err != nil {
				t.Fatalf("SA list pods (phase 2, iter %d): %v", i, err)
			}
		}
	})

	// After pod restart, the in-memory aggregator starts fresh, so EventsProcessed
	// reflects only what the new pipeline run processed (not cumulative).
	// We verify the checkpoint worked by checking:
	// 1. The report still exists and has observed rules (pipeline recovered)
	// 2. The FileOffset advanced past the pre-restart checkpoint
	// 3. The new EventsProcessed is small (not a full replay from offset 0)

	// Wait for the new pipeline to process events and update the report.
	report = waitForPolicyReportCondition(ctx, t, reportName, ns, func(r *audiciav1alpha1.AudiciaPolicyReport) bool {
		return r.Status.EventsProcessed > 0 && r.Status.LastProcessedTime != nil
	}, defaultTimeout)
	t.Logf("phase 2: %d events processed by new pipeline", report.Status.EventsProcessed)

	// The report should still have observed rules (data isn't lost).
	if len(report.Status.ObservedRules) == 0 {
		t.Error("expected observed rules after restart, got none")
	}
	assertRuleExists(t, report.Status.ObservedRules, "", "pods", "list")

	// Wait for the new checkpoint to flush and verify FileOffset advanced.
	time.Sleep(12 * time.Second)
	src = getAudiciaSource(ctx, t, sourceName, ns)
	if src.Status.FileOffset < checkpointOffset {
		t.Errorf("FileOffset went backwards: %d < %d (checkpoint not restored)", src.Status.FileOffset, checkpointOffset)
	}
	t.Logf("post-restart fileOffset: %d (was %d)", src.Status.FileOffset, checkpointOffset)

	// If the operator replayed from 0, EventsProcessed would be very large
	// (all audit events in the file). With checkpoint, it should be small.
	if report.Status.EventsProcessed > 1000 {
		t.Errorf("EventsProcessed=%d is suspiciously high, suggests full replay from offset 0",
			report.Status.EventsProcessed)
	}

	t.Logf("checkpoint test passed: fileOffset %d -> %d, events=%d",
		checkpointOffset, src.Status.FileOffset, report.Status.EventsProcessed)
}
