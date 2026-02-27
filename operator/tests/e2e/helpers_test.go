//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	authv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	sigsyaml "sigs.k8s.io/yaml"

	audiciav1alpha1 "github.com/felixnotka/audicia/operator/pkg/apis/audicia.io/v1alpha1"
)

const (
	defaultTimeout = 3 * time.Minute
	pollInterval   = 2 * time.Second
	auditLogPath   = "/var/log/kubernetes/audit/audit.log"
)

// restConfig holds the rest.Config for SA token impersonation.
// Set in buildClients().
var restConfig *rest.Config

// createNamespace creates a namespace and registers cleanup.
func createNamespace(ctx context.Context, t *testing.T, name string) {
	t.Helper()
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: name},
	}
	if err := k8sClient.Create(ctx, ns); err != nil {
		t.Fatalf("create namespace %s: %v", name, err)
	}
	t.Cleanup(func() {
		_ = k8sClient.Delete(context.Background(), ns)
	})
}

// createServiceAccount creates a ServiceAccount in the given namespace.
func createServiceAccount(ctx context.Context, t *testing.T, name, ns string) {
	t.Helper()
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
	}
	if err := k8sClient.Create(ctx, sa); err != nil {
		t.Fatalf("create SA %s/%s: %v", ns, name, err)
	}
}

// createRole creates a Role with the given PolicyRules.
func createRole(ctx context.Context, t *testing.T, name, ns string, rules []rbacv1.PolicyRule) {
	t.Helper()
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Rules:      rules,
	}
	if err := k8sClient.Create(ctx, role); err != nil {
		t.Fatalf("create role %s/%s: %v", ns, name, err)
	}
}

// createClusterRole creates a ClusterRole with the given PolicyRules.
func createClusterRole(ctx context.Context, t *testing.T, name string, rules []rbacv1.PolicyRule) {
	t.Helper()
	cr := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Rules:      rules,
	}
	if err := k8sClient.Create(ctx, cr); err != nil {
		t.Fatalf("create clusterrole %s: %v", name, err)
	}
	t.Cleanup(func() {
		_ = k8sClient.Delete(context.Background(), cr)
	})
}

// bindRoleToSA creates a RoleBinding binding a Role to a ServiceAccount.
func bindRoleToSA(ctx context.Context, t *testing.T, roleName, saName, saNamespace, bindingNS, bindingName string) {
	t.Helper()
	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: bindingName, Namespace: bindingNS},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     roleName,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      saName,
				Namespace: saNamespace,
			},
		},
	}
	if err := k8sClient.Create(ctx, rb); err != nil {
		t.Fatalf("create rolebinding %s/%s: %v", bindingNS, bindingName, err)
	}
}

// bindClusterRoleToSA creates a ClusterRoleBinding binding a ClusterRole to a ServiceAccount.
func bindClusterRoleToSA(ctx context.Context, t *testing.T, crName, saName, saNamespace, bindingName string) {
	t.Helper()
	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: bindingName},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     crName,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      saName,
				Namespace: saNamespace,
			},
		},
	}
	if err := k8sClient.Create(ctx, crb); err != nil {
		t.Fatalf("create clusterrolebinding %s: %v", bindingName, err)
	}
	t.Cleanup(func() {
		_ = k8sClient.Delete(context.Background(), crb)
	})
}

// bindClusterRoleToSAViaRoleBinding creates a RoleBinding that references a ClusterRole,
// scoping the ClusterRole's permissions to the binding's namespace.
func bindClusterRoleToSAViaRoleBinding(ctx context.Context, t *testing.T, crName, saName, saNamespace, bindingNS, bindingName string) {
	t.Helper()
	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: bindingName, Namespace: bindingNS},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     crName,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      saName,
				Namespace: saNamespace,
			},
		},
	}
	if err := k8sClient.Create(ctx, rb); err != nil {
		t.Fatalf("create rolebinding %s/%s: %v", bindingNS, bindingName, err)
	}
}

// actAsServiceAccount gets a token for the SA via TokenRequest and executes
// the given function with a clientset authenticated as that SA. This generates
// real audit events attributed to system:serviceaccount:<ns>:<name>.
func actAsServiceAccount(ctx context.Context, t *testing.T, saName, saNamespace string, actions func(*kubernetes.Clientset)) {
	t.Helper()

	tokenReq := &authv1.TokenRequest{
		Spec: authv1.TokenRequestSpec{
			ExpirationSeconds: ptr.To[int64](3600),
		},
	}

	tokenResp, err := clientset.CoreV1().ServiceAccounts(saNamespace).
		CreateToken(ctx, saName, tokenReq, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create token for SA %s/%s: %v", saNamespace, saName, err)
	}

	cfg := rest.CopyConfig(restConfig)
	cfg.BearerToken = tokenResp.Status.Token
	// Clear client cert auth so the bearer token is used.
	cfg.CertFile = ""
	cfg.KeyFile = ""
	cfg.CertData = nil
	cfg.KeyData = nil

	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		t.Fatalf("create clientset with SA token: %v", err)
	}

	actions(cs)
}

// createAudiciaSource creates an AudiciaSource CR configured for file-based
// ingestion with fast checkpoint settings for testing.
func createAudiciaSource(ctx context.Context, t *testing.T, name, ns string, spec *audiciav1alpha1.AudiciaSourceSpec) {
	t.Helper()

	if spec == nil {
		spec = &audiciav1alpha1.AudiciaSourceSpec{}
	}
	if spec.SourceType == "" {
		spec.SourceType = audiciav1alpha1.SourceTypeK8sAuditLog
	}
	if spec.SourceType == audiciav1alpha1.SourceTypeK8sAuditLog && spec.Location == nil {
		spec.Location = &audiciav1alpha1.FileLocation{Path: auditLogPath}
	}
	spec.IgnoreSystemUsers = true
	if spec.Checkpoint.IntervalSeconds == 0 {
		spec.Checkpoint.IntervalSeconds = 5
	}
	if spec.Checkpoint.BatchSize == 0 {
		spec.Checkpoint.BatchSize = 100
	}

	src := &audiciav1alpha1.AudiciaSource{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec:       *spec,
	}
	if err := k8sClient.Create(ctx, src); err != nil {
		t.Fatalf("create AudiciaSource %s/%s: %v", ns, name, err)
	}
	t.Cleanup(func() {
		_ = k8sClient.Delete(context.Background(), src)
	})
}

// waitForPolicyReport polls until an AudiciaPolicyReport exists with at least one ObservedRule.
func waitForPolicyReport(ctx context.Context, t *testing.T, name, ns string, timeout time.Duration) *audiciav1alpha1.AudiciaPolicyReport {
	t.Helper()
	return waitForPolicyReportCondition(ctx, t, name, ns, func(r *audiciav1alpha1.AudiciaPolicyReport) bool {
		return len(r.Status.ObservedRules) > 0
	}, timeout)
}

// waitForPolicyReportCondition polls until the report satisfies the condition function.
func waitForPolicyReportCondition(ctx context.Context, t *testing.T, name, ns string, condFn func(*audiciav1alpha1.AudiciaPolicyReport) bool, timeout time.Duration) *audiciav1alpha1.AudiciaPolicyReport {
	t.Helper()

	var report audiciav1alpha1.AudiciaPolicyReport
	err := wait.PollUntilContextTimeout(ctx, pollInterval, timeout, true, func(ctx context.Context) (bool, error) {
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: ns}, &report); err != nil {
			return false, nil // not found yet
		}
		return condFn(&report), nil
	})
	if err != nil {
		t.Fatalf("timed out waiting for AudiciaPolicyReport %s/%s: %v", ns, name, err)
	}
	return &report
}

// expectedReportName returns the report name the controller generates for a given SA name.
func expectedReportName(saName string) string {
	return "report-" + sanitizeName(saName)
}

// sanitizeName mirrors the controller's sanitization logic.
func sanitizeName(name string) string {
	s := strings.ToLower(name)
	s = strings.ReplaceAll(s, "@", "-at-")
	s = strings.ReplaceAll(s, ":", "-")
	s = strings.ReplaceAll(s, "/", "-")
	s = strings.ReplaceAll(s, ".", "-")
	if len(s) > 63 {
		s = s[:63]
	}
	s = strings.TrimRight(s, "-")
	return s
}

// assertRuleExists checks that at least one ObservedRule matches the given criteria.
func assertRuleExists(t *testing.T, rules []audiciav1alpha1.ObservedRule, apiGroup, resource, verb string) {
	t.Helper()
	for _, r := range rules {
		if containsStr(r.APIGroups, apiGroup) && containsStr(r.Resources, resource) && containsStr(r.Verbs, verb) {
			return
		}
	}
	t.Errorf("expected rule {apiGroup=%q, resource=%q, verb=%q} not found in %d rules", apiGroup, resource, verb, len(rules))
}

// assertNoRuleWithNamespace checks that no ObservedRule has the given namespace.
func assertNoRuleWithNamespace(t *testing.T, rules []audiciav1alpha1.ObservedRule, ns string) {
	t.Helper()
	for _, r := range rules {
		if r.Namespace == ns {
			t.Errorf("found unexpected rule in namespace %q: %+v", ns, r)
		}
	}
}

// deleteOperatorPod deletes the operator pod and waits for the deployment to recover.
func deleteOperatorPod(ctx context.Context, t *testing.T) {
	t.Helper()

	deployName := helmFullName
	pods, err := clientset.CoreV1().Pods(helmNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app.kubernetes.io/name=audicia-operator,app.kubernetes.io/instance=%s", helmReleaseName),
	})
	if err != nil {
		t.Fatalf("list operator pods: %v", err)
	}
	for i := range pods.Items {
		if err := clientset.CoreV1().Pods(helmNamespace).Delete(ctx, pods.Items[i].Name, metav1.DeleteOptions{}); err != nil {
			t.Logf("warning: failed to delete pod %s: %v", pods.Items[i].Name, err)
		}
	}

	// Wait for deployment to recover.
	err = wait.PollUntilContextTimeout(ctx, 2*time.Second, 2*time.Minute, true, func(ctx context.Context) (bool, error) {
		var dep appsv1.Deployment
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: deployName, Namespace: helmNamespace}, &dep); err != nil {
			return false, nil
		}
		return dep.Status.ReadyReplicas > 0 && dep.Status.ReadyReplicas == *dep.Spec.Replicas, nil
	})
	if err != nil {
		t.Fatalf("operator deployment did not recover: %v", err)
	}
}

// getAudiciaSource fetches the current state of an AudiciaSource.
func getAudiciaSource(ctx context.Context, t *testing.T, name, ns string) *audiciav1alpha1.AudiciaSource {
	t.Helper()
	var src audiciav1alpha1.AudiciaSource
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: ns}, &src); err != nil {
		t.Fatalf("get AudiciaSource %s/%s: %v", ns, name, err)
	}
	return &src
}

// containsStr checks if a slice contains a string.
func containsStr(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

// uniqueSuffix returns a short unique suffix based on the current time.
func uniqueSuffix() string {
	return fmt.Sprintf("%x", time.Now().UnixNano()%0xFFFFFF)
}

// waitForSource polls until the AudiciaSource status matches a condition.
func waitForSource(ctx context.Context, t *testing.T, name, ns string, condFn func(*audiciav1alpha1.AudiciaSource) bool, timeout time.Duration) *audiciav1alpha1.AudiciaSource {
	t.Helper()
	var src audiciav1alpha1.AudiciaSource
	err := wait.PollUntilContextTimeout(ctx, pollInterval, timeout, true, func(ctx context.Context) (bool, error) {
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: ns}, &src); err != nil {
			return false, nil
		}
		return condFn(&src), nil
	})
	if err != nil {
		t.Fatalf("timed out waiting for AudiciaSource %s/%s condition: %v", ns, name, err)
	}
	return &src
}

// getPolicyReport fetches the current state of an AudiciaPolicyReport.
func getPolicyReport(ctx context.Context, t *testing.T, name, ns string) *audiciav1alpha1.AudiciaPolicyReport {
	t.Helper()
	var report audiciav1alpha1.AudiciaPolicyReport
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: ns}, &report); err != nil {
		t.Fatalf("get AudiciaPolicyReport %s/%s: %v", ns, name, err)
	}
	return &report
}

// listPolicyReports lists all AudiciaPolicyReports in a namespace.
func listPolicyReports(ctx context.Context, t *testing.T, ns string) []audiciav1alpha1.AudiciaPolicyReport {
	t.Helper()
	var list audiciav1alpha1.AudiciaPolicyReportList
	if err := k8sClient.List(ctx, &list, client.InNamespace(ns)); err != nil {
		t.Fatalf("list AudiciaPolicyReports in %s: %v", ns, err)
	}
	return list.Items
}

// parseRoleFromManifests decodes YAML manifests and returns the first Role found.
func parseRoleFromManifests(t *testing.T, manifests []string) *rbacv1.Role {
	t.Helper()
	for _, m := range manifests {
		var role rbacv1.Role
		if err := sigsyaml.Unmarshal([]byte(m), &role); err != nil {
			continue
		}
		if role.Kind == "Role" && len(role.Rules) > 0 {
			return &role
		}
	}
	t.Fatal("no Role found in manifests")
	return nil
}

// startPortForward starts kubectl port-forward as a background process and waits
// until the local port is reachable. Returns a cancel function to stop it.
func startPortForward(ctx context.Context, t *testing.T, target, remotePort, localPort string) func() {
	t.Helper()

	pfCtx, pfCancel := context.WithCancel(ctx)
	pfCmd := exec.CommandContext(pfCtx, "kubectl", "port-forward",
		"-n", helmNamespace,
		target,
		localPort+":"+remotePort,
		"--context", "kind-"+kindClusterName)

	var pfStderr bytes.Buffer
	pfCmd.Stderr = &pfStderr
	if err := pfCmd.Start(); err != nil {
		pfCancel()
		t.Fatalf("start port-forward to %s: %v", target, err)
	}

	cancel := func() {
		pfCancel()
		_ = pfCmd.Wait()
	}
	t.Cleanup(cancel)

	// Wait for port to become reachable.
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := http.Get(fmt.Sprintf("http://localhost:%s/", localPort))
		if err == nil {
			conn.Body.Close()
			return cancel
		}
		time.Sleep(500 * time.Millisecond)
	}

	// Return cancel even if not reachable yet; caller's HTTP client will retry.
	return cancel
}

// assertCondition verifies that a condition with the given type, reason, and status exists.
func assertCondition(t *testing.T, conditions []metav1.Condition, condType, reason string, status metav1.ConditionStatus) {
	t.Helper()
	cond := meta.FindStatusCondition(conditions, condType)
	if cond == nil {
		t.Errorf("expected condition %q, but not found", condType)
		return
	}
	if cond.Status != status {
		t.Errorf("condition %q: expected status=%s, got %s", condType, status, cond.Status)
	}
	if reason != "" && cond.Reason != reason {
		t.Errorf("condition %q: expected reason=%q, got %q", condType, reason, cond.Reason)
	}
}

// assertMetricExists checks that a Prometheus metric line exists in the body.
func assertMetricExists(t *testing.T, body, name string) {
	t.Helper()
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, name+"{") || strings.HasPrefix(line, name+" ") {
			return
		}
	}
	t.Errorf("metric %q not found in /metrics output", name)
}

// assertMetricPositive checks that a Prometheus metric has at least one sample with value > 0.
// Handles both labeled (name{labels} value) and unlabeled (name value) formats.
func assertMetricPositive(t *testing.T, body, name string) {
	t.Helper()
	re := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(name) + `(?:\{[^}]*\})?\s+(\S+)$`)
	matches := re.FindAllStringSubmatch(body, -1)
	if len(matches) == 0 {
		t.Errorf("metric %q not found in /metrics output", name)
		return
	}
	for _, m := range matches {
		val, err := strconv.ParseFloat(m[1], 64)
		if err != nil {
			continue
		}
		if val > 0 {
			return
		}
	}
	t.Errorf("metric %q exists but no sample has value > 0", name)
}

// postAuditEventsRaw POSTs a raw JSON body to the webhook and returns the response.
// Unlike postAuditEvents, it does not fail on non-200 status codes.
func postAuditEventsRaw(t *testing.T, httpClient *http.Client, url string, body []byte) *http.Response {
	t.Helper()
	resp, err := httpClient.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST to webhook: %v", err)
	}
	// Drain and close body so the connection can be reused.
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return resp
}
