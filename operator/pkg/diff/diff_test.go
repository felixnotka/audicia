package diff

import (
	"sort"
	"testing"

	audiciav1alpha1 "github.com/felixnotka/audicia/operator/pkg/apis/audicia.io/v1alpha1"
	"github.com/felixnotka/audicia/operator/pkg/rbac"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func obs(apiGroup, resource, verb, ns string) audiciav1alpha1.ObservedRule {
	return audiciav1alpha1.ObservedRule{
		APIGroups: []string{apiGroup},
		Resources: []string{resource},
		Verbs:     []string{verb},
		Namespace: ns,
		FirstSeen: metav1.Now(),
		LastSeen:  metav1.Now(),
		Count:     1,
	}
}

func obsNonResource(url, verb string) audiciav1alpha1.ObservedRule {
	return audiciav1alpha1.ObservedRule{
		NonResourceURLs: []string{url},
		Verbs:           []string{verb},
		FirstSeen:       metav1.Now(),
		LastSeen:        metav1.Now(),
		Count:           1,
	}
}

func eff(apiGroup, resource string, verbs []string, ns string) rbac.ScopedRule {
	return rbac.ScopedRule{
		PolicyRule: rbacv1.PolicyRule{
			APIGroups: []string{apiGroup},
			Resources: []string{resource},
			Verbs:     verbs,
		},
		Namespace: ns,
	}
}

func effNonResource(url string, verbs []string) rbac.ScopedRule {
	return rbac.ScopedRule{
		PolicyRule: rbacv1.PolicyRule{
			NonResourceURLs: []string{url},
			Verbs:           verbs,
		},
	}
}

func effWithResourceNames(apiGroup, resource string, verbs, resourceNames []string, ns string) rbac.ScopedRule {
	return rbac.ScopedRule{
		PolicyRule: rbacv1.PolicyRule{
			APIGroups:     []string{apiGroup},
			Resources:     []string{resource},
			Verbs:         verbs,
			ResourceNames: resourceNames,
		},
		Namespace: ns,
	}
}

func TestEvaluate_BothEmpty(t *testing.T) {
	report := Evaluate(nil, nil)
	if report == nil {
		t.Fatal("expected non-nil report for empty inputs")
	}
	if report.Score != 100 {
		t.Errorf("expected score 100, got %d", report.Score)
	}
	if report.Severity != audiciav1alpha1.ComplianceSeverityGreen {
		t.Errorf("expected Green severity, got %s", report.Severity)
	}
}

func TestEvaluate_NoEffective_WithObserved(t *testing.T) {
	observed := []audiciav1alpha1.ObservedRule{obs("", "pods", "get", "default")}
	report := Evaluate(observed, nil)
	if report != nil {
		t.Error("expected nil report when no effective rules and observed rules exist")
	}
}

func TestEvaluate_PerfectMatch(t *testing.T) {
	observed := []audiciav1alpha1.ObservedRule{
		obs("", "pods", "get", "default"),
		obs("", "pods", "list", "default"),
	}
	effective := []rbac.ScopedRule{
		eff("", "pods", []string{"get", "list"}, "default"),
	}

	report := Evaluate(observed, effective)
	if report == nil {
		t.Fatal("expected non-nil report")
	}
	if report.Score != 100 {
		t.Errorf("expected score 100, got %d", report.Score)
	}
	if report.Severity != audiciav1alpha1.ComplianceSeverityGreen {
		t.Errorf("expected Green, got %s", report.Severity)
	}
	if report.ExcessCount != 0 {
		t.Errorf("expected 0 excess, got %d", report.ExcessCount)
	}
	if report.UncoveredCount != 0 {
		t.Errorf("expected 0 uncovered, got %d", report.UncoveredCount)
	}
}

func TestEvaluate_SignificantExcess_Red(t *testing.T) {
	// 1 observed rule using 1 effective rule, but 9 excess effective rules.
	observed := []audiciav1alpha1.ObservedRule{
		obs("", "pods", "get", "default"),
	}
	effective := []rbac.ScopedRule{
		eff("", "pods", []string{"get"}, "default"),            // used
		eff("", "secrets", []string{"get"}, "default"),         // excess, sensitive
		eff("", "nodes", []string{"get"}, ""),                  // excess, sensitive
		eff("", "deployments", []string{"get"}, "default"),     // excess
		eff("", "services", []string{"get"}, "default"),        // excess
		eff("", "configmaps", []string{"get"}, "default"),      // excess
		eff("", "daemonsets", []string{"get"}, "default"),      // excess
		eff("", "replicasets", []string{"get"}, "default"),     // excess
		eff("", "statefulsets", []string{"get"}, "default"),    // excess
		eff("apps", "deployments", []string{"get"}, "default"), // excess
	}

	report := Evaluate(observed, effective)
	if report == nil {
		t.Fatal("expected non-nil report")
	}
	// 1 covered, 9 excess → score = 1/10*100 = 10
	if report.Score != 10 {
		t.Errorf("expected score 10, got %d", report.Score)
	}
	if report.Severity != audiciav1alpha1.ComplianceSeverityRed {
		t.Errorf("expected Red, got %s", report.Severity)
	}
	if report.ExcessCount != 9 {
		t.Errorf("expected 9 excess, got %d", report.ExcessCount)
	}
	if len(report.SensitiveExcess) < 2 {
		t.Errorf("expected at least 2 sensitive excess entries, got %d: %v", len(report.SensitiveExcess), report.SensitiveExcess)
	}
}

func TestEvaluate_ModerateExcess_Red(t *testing.T) {
	// 4 effective rules, only 1 used → 1/4*100 = 25 → Red.
	// The 3 observed actions all hit the same effective rule (pods get/list/watch).
	observed := []audiciav1alpha1.ObservedRule{
		obs("", "pods", "get", "default"),
		obs("", "pods", "list", "default"),
		obs("", "pods", "watch", "default"),
	}
	effective := []rbac.ScopedRule{
		eff("", "pods", []string{"get", "list", "watch"}, "default"),       // used
		eff("", "deployments", []string{"get"}, "default"),                 // excess
		eff("", "services", []string{"get"}, "default"),                    // excess
		eff("", "configmaps", []string{"get", "list", "watch"}, "default"), // excess
	}

	report := Evaluate(observed, effective)
	if report == nil {
		t.Fatal("expected non-nil report")
	}
	// 1 used effective / 4 total effective → 25 → Red
	if report.Score != 25 {
		t.Errorf("expected score 25, got %d", report.Score)
	}
	if report.Severity != audiciav1alpha1.ComplianceSeverityRed {
		t.Errorf("expected Red, got %s", report.Severity)
	}
}

func TestEvaluate_ClusterWideCoversAllNamespaces(t *testing.T) {
	// Cluster-wide effective rule (namespace="") should cover observed in any namespace.
	observed := []audiciav1alpha1.ObservedRule{
		obs("", "pods", "get", "kube-system"),
		obs("", "pods", "get", "monitoring"),
	}
	effective := []rbac.ScopedRule{
		eff("", "pods", []string{"get"}, ""), // cluster-wide
	}

	report := Evaluate(observed, effective)
	if report == nil {
		t.Fatal("expected non-nil report")
	}
	if report.UncoveredCount != 0 {
		t.Errorf("expected 0 uncovered, got %d", report.UncoveredCount)
	}
	if report.Score != 100 {
		t.Errorf("expected score 100, got %d", report.Score)
	}
}

func TestEvaluate_NamespaceScopedDoesNotCrossNamespace(t *testing.T) {
	// Effective rule scoped to "default" should NOT cover observed in "kube-system".
	observed := []audiciav1alpha1.ObservedRule{
		obs("", "pods", "get", "kube-system"),
	}
	effective := []rbac.ScopedRule{
		eff("", "pods", []string{"get"}, "default"),
	}

	report := Evaluate(observed, effective)
	if report == nil {
		t.Fatal("expected non-nil report")
	}
	if report.UncoveredCount != 1 {
		t.Errorf("expected 1 uncovered, got %d", report.UncoveredCount)
	}
}

func TestEvaluate_WildcardVerb(t *testing.T) {
	// Effective rule with verb "*" should cover any observed verb.
	observed := []audiciav1alpha1.ObservedRule{
		obs("", "pods", "get", "default"),
		obs("", "pods", "delete", "default"),
	}
	effective := []rbac.ScopedRule{
		eff("", "pods", []string{"*"}, "default"),
	}

	report := Evaluate(observed, effective)
	if report == nil {
		t.Fatal("expected non-nil report")
	}
	if report.UncoveredCount != 0 {
		t.Errorf("expected 0 uncovered, got %d", report.UncoveredCount)
	}
	if report.Score != 100 {
		t.Errorf("expected score 100, got %d", report.Score)
	}
}

func TestEvaluate_WildcardResource(t *testing.T) {
	observed := []audiciav1alpha1.ObservedRule{
		obs("", "pods", "get", "default"),
		obs("", "services", "get", "default"),
	}
	effective := []rbac.ScopedRule{
		eff("", "*", []string{"get"}, "default"),
	}

	report := Evaluate(observed, effective)
	if report == nil {
		t.Fatal("expected non-nil report")
	}
	if report.UncoveredCount != 0 {
		t.Errorf("expected 0 uncovered, got %d", report.UncoveredCount)
	}
}

func TestEvaluate_WildcardAPIGroup(t *testing.T) {
	observed := []audiciav1alpha1.ObservedRule{
		obs("apps", "deployments", "get", "default"),
		obs("batch", "jobs", "get", "default"),
	}
	effective := []rbac.ScopedRule{
		eff("*", "deployments", []string{"get"}, "default"),
		eff("*", "jobs", []string{"get"}, "default"),
	}

	report := Evaluate(observed, effective)
	if report == nil {
		t.Fatal("expected non-nil report")
	}
	if report.UncoveredCount != 0 {
		t.Errorf("expected 0 uncovered, got %d", report.UncoveredCount)
	}
}

func TestEvaluate_ResourceNamesNotCovering(t *testing.T) {
	// Effective rules with ResourceNames should NOT cover general observed rules.
	observed := []audiciav1alpha1.ObservedRule{
		obs("", "configmaps", "get", "default"),
	}
	effective := []rbac.ScopedRule{
		effWithResourceNames("", "configmaps", []string{"get"}, []string{"my-config"}, "default"),
	}

	report := Evaluate(observed, effective)
	if report == nil {
		t.Fatal("expected non-nil report")
	}
	if report.UncoveredCount != 1 {
		t.Errorf("expected 1 uncovered, got %d", report.UncoveredCount)
	}
}

func TestEvaluate_NonResourceURLs(t *testing.T) {
	observed := []audiciav1alpha1.ObservedRule{
		obsNonResource("/metrics", "get"),
		obsNonResource("/healthz", "get"),
	}
	effective := []rbac.ScopedRule{
		effNonResource("/metrics", []string{"get"}),
		effNonResource("/healthz", []string{"get"}),
		effNonResource("/readyz", []string{"get"}), // excess
	}

	report := Evaluate(observed, effective)
	if report == nil {
		t.Fatal("expected non-nil report")
	}
	if report.UncoveredCount != 0 {
		t.Errorf("expected 0 uncovered, got %d", report.UncoveredCount)
	}
	if report.ExcessCount != 1 {
		t.Errorf("expected 1 excess, got %d", report.ExcessCount)
	}
	// 2 used, 1 excess → 2/3*100 = 66
	if report.Score != 66 {
		t.Errorf("expected score 66, got %d", report.Score)
	}
	if report.Severity != audiciav1alpha1.ComplianceSeverityYellow {
		t.Errorf("expected Yellow, got %s", report.Severity)
	}
}

func TestEvaluate_SensitiveExcessDetection(t *testing.T) {
	observed := []audiciav1alpha1.ObservedRule{
		obs("", "pods", "get", "default"),
	}
	effective := []rbac.ScopedRule{
		eff("", "pods", []string{"get"}, "default"),                        // used
		eff("", "secrets", []string{"get", "list"}, "default"),             // excess, sensitive
		eff("", "mutatingwebhookconfigurations", []string{"get"}, ""),      // excess, sensitive
		eff("", "validatingwebhookconfigurations", []string{"create"}, ""), // excess, sensitive
		eff("", "customresourcedefinitions", []string{"get"}, ""),          // excess, sensitive
	}

	report := Evaluate(observed, effective)
	if report == nil {
		t.Fatal("expected non-nil report")
	}
	if len(report.SensitiveExcess) != 4 {
		t.Errorf("expected 4 sensitive excess entries, got %d: %v", len(report.SensitiveExcess), report.SensitiveExcess)
	}
}

func TestEvaluate_WildcardResourceSensitive(t *testing.T) {
	// Wildcard resource in excess should flag "* (all resources)".
	observed := []audiciav1alpha1.ObservedRule{
		obs("", "pods", "get", "default"),
	}
	effective := []rbac.ScopedRule{
		eff("", "pods", []string{"get"}, "default"),  // used
		eff("", "*", []string{"get"}, "kube-system"), // excess, wildcard
	}

	report := Evaluate(observed, effective)
	if report == nil {
		t.Fatal("expected non-nil report")
	}
	found := false
	for _, s := range report.SensitiveExcess {
		if s == "* (all resources)" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected '* (all resources)' in sensitive excess, got %v", report.SensitiveExcess)
	}
}

func TestEvaluate_NoObserved_AllExcess(t *testing.T) {
	// All effective rules are excess, nothing observed → score = 0.
	effective := []rbac.ScopedRule{
		eff("", "pods", []string{"get"}, "default"),
		eff("", "secrets", []string{"get"}, "default"),
	}

	report := Evaluate(nil, effective)
	if report == nil {
		t.Fatal("expected non-nil report")
	}
	if report.Score != 0 {
		t.Errorf("expected score 0, got %d", report.Score)
	}
	if report.Severity != audiciav1alpha1.ComplianceSeverityRed {
		t.Errorf("expected Red, got %s", report.Severity)
	}
	if report.ExcessCount != 2 {
		t.Errorf("expected 2 excess, got %d", report.ExcessCount)
	}
}

func TestSeverityFromScore(t *testing.T) {
	tests := []struct {
		score    int32
		expected audiciav1alpha1.ComplianceSeverity
	}{
		{100, audiciav1alpha1.ComplianceSeverityGreen},
		{80, audiciav1alpha1.ComplianceSeverityGreen},
		{79, audiciav1alpha1.ComplianceSeverityYellow},
		{50, audiciav1alpha1.ComplianceSeverityYellow},
		{49, audiciav1alpha1.ComplianceSeverityRed},
		{0, audiciav1alpha1.ComplianceSeverityRed},
	}

	for _, tt := range tests {
		got := severityFromScore(tt.score)
		if got != tt.expected {
			t.Errorf("severityFromScore(%d) = %s, want %s", tt.score, got, tt.expected)
		}
	}
}

func TestSliceCovers(t *testing.T) {
	tests := []struct {
		name     string
		granted  []string
		required []string
		want     bool
	}{
		{"exact match", []string{"get", "list"}, []string{"get", "list"}, true},
		{"superset", []string{"get", "list", "watch"}, []string{"get"}, true},
		{"missing verb", []string{"get"}, []string{"get", "list"}, false},
		{"wildcard", []string{"*"}, []string{"get", "list", "delete"}, true},
		{"empty required", []string{"get"}, nil, true},
		{"empty granted", nil, []string{"get"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sliceCovers(tt.granted, tt.required)
			if got != tt.want {
				t.Errorf("sliceCovers(%v, %v) = %v, want %v", tt.granted, tt.required, got, tt.want)
			}
		})
	}
}

func TestEvaluate_MixedResourceAndNonResource(t *testing.T) {
	observed := []audiciav1alpha1.ObservedRule{
		obs("", "pods", "get", "default"),
		obsNonResource("/metrics", "get"),
	}
	effective := []rbac.ScopedRule{
		eff("", "pods", []string{"get"}, "default"),
		effNonResource("/metrics", []string{"get"}),
		effNonResource("/healthz", []string{"get"}), // excess
	}

	report := Evaluate(observed, effective)
	if report == nil {
		t.Fatal("expected non-nil report")
	}
	if report.UsedCount != 2 {
		t.Errorf("expected 2 covered, got %d", report.UsedCount)
	}
	if report.ExcessCount != 1 {
		t.Errorf("expected 1 excess, got %d", report.ExcessCount)
	}
	if report.UncoveredCount != 0 {
		t.Errorf("expected 0 uncovered, got %d", report.UncoveredCount)
	}
	// 2 used, 1 excess → 2/3*100 = 66
	if report.Score != 66 {
		t.Errorf("expected score 66, got %d", report.Score)
	}
}

// --- classifyEffective ---

func TestClassifyEffective_AllUsed(t *testing.T) {
	effective := []rbac.ScopedRule{
		eff("", "pods", []string{"get"}, "default"),
		eff("", "services", []string{"list"}, "default"),
	}
	used := []bool{true, true}

	usedCount, excessCount, sensitive := classifyEffective(effective, used)
	if usedCount != 2 {
		t.Errorf("usedCount = %d, want 2", usedCount)
	}
	if excessCount != 0 {
		t.Errorf("excessCount = %d, want 0", excessCount)
	}
	if len(sensitive) != 0 {
		t.Errorf("sensitive = %v, want empty", sensitive)
	}
}

func TestClassifyEffective_AllExcess(t *testing.T) {
	effective := []rbac.ScopedRule{
		eff("", "pods", []string{"get"}, "default"),
		eff("", "secrets", []string{"get"}, "default"),
	}
	used := []bool{false, false}

	usedCount, excessCount, sensitive := classifyEffective(effective, used)
	if usedCount != 0 {
		t.Errorf("usedCount = %d, want 0", usedCount)
	}
	if excessCount != 2 {
		t.Errorf("excessCount = %d, want 2", excessCount)
	}
	if len(sensitive) != 1 || sensitive[0] != "secrets" {
		t.Errorf("sensitive = %v, want [secrets]", sensitive)
	}
}

func TestClassifyEffective_Mixed(t *testing.T) {
	effective := []rbac.ScopedRule{
		eff("", "pods", []string{"get"}, "default"),
		eff("", "secrets", []string{"get"}, "default"),
		eff("", "nodes", []string{"list"}, ""),
	}
	used := []bool{true, false, false}

	usedCount, excessCount, sensitive := classifyEffective(effective, used)
	if usedCount != 1 {
		t.Errorf("usedCount = %d, want 1", usedCount)
	}
	if excessCount != 2 {
		t.Errorf("excessCount = %d, want 2", excessCount)
	}
	sort.Strings(sensitive)
	if len(sensitive) != 2 || sensitive[0] != "nodes" || sensitive[1] != "secrets" {
		t.Errorf("sensitive = %v, want [nodes, secrets]", sensitive)
	}
}

func TestClassifyEffective_Empty(t *testing.T) {
	usedCount, excessCount, sensitive := classifyEffective(nil, nil)
	if usedCount != 0 || excessCount != 0 || len(sensitive) != 0 {
		t.Errorf("expected all zeros for empty input, got used=%d excess=%d sensitive=%v",
			usedCount, excessCount, sensitive)
	}
}

// --- collectSensitive ---

func TestCollectSensitive_KnownSensitive(t *testing.T) {
	seen := make(map[string]bool)
	var out []string
	collectSensitive([]string{"secrets", "configmaps", "nodes"}, seen, &out)
	sort.Strings(out)
	if len(out) != 2 || out[0] != "nodes" || out[1] != "secrets" {
		t.Errorf("got %v, want [nodes, secrets]", out)
	}
}

func TestCollectSensitive_Wildcard(t *testing.T) {
	seen := make(map[string]bool)
	var out []string
	collectSensitive([]string{"*"}, seen, &out)
	if len(out) != 1 || out[0] != "* (all resources)" {
		t.Errorf("got %v, want [* (all resources)]", out)
	}
}

func TestCollectSensitive_NoDuplicates(t *testing.T) {
	seen := make(map[string]bool)
	var out []string
	collectSensitive([]string{"secrets", "secrets", "secrets"}, seen, &out)
	if len(out) != 1 {
		t.Errorf("got %d entries, want 1 (no duplicates)", len(out))
	}
}

func TestCollectSensitive_NonSensitiveIgnored(t *testing.T) {
	seen := make(map[string]bool)
	var out []string
	collectSensitive([]string{"pods", "configmaps", "deployments"}, seen, &out)
	if len(out) != 0 {
		t.Errorf("got %v, want empty (no sensitive resources)", out)
	}
}

func TestCollectSensitive_CaseInsensitive(t *testing.T) {
	seen := make(map[string]bool)
	var out []string
	collectSensitive([]string{"Secrets", "NODES"}, seen, &out)
	sort.Strings(out)
	if len(out) != 2 || out[0] != "nodes" || out[1] != "secrets" {
		t.Errorf("got %v, want [nodes, secrets]", out)
	}
}

// --- markUsed ---

func TestMarkUsed_ResourceRules(t *testing.T) {
	effective := []rbac.ScopedRule{
		eff("", "pods", []string{"get"}, "default"),
		eff("", "services", []string{"get"}, "default"),
		eff("", "secrets", []string{"get"}, "default"),
	}
	used := make([]bool, 3)

	markUsed(obs("", "pods", "get", "default"), effective, used)
	if !used[0] || used[1] || used[2] {
		t.Errorf("used = %v, want [true, false, false]", used)
	}
}

func TestMarkUsed_NonResourceURLs(t *testing.T) {
	effective := []rbac.ScopedRule{
		effNonResource("/metrics", []string{"get"}),
		effNonResource("/healthz", []string{"get"}),
	}
	used := make([]bool, 2)

	markUsed(obsNonResource("/metrics", "get"), effective, used)
	if !used[0] || used[1] {
		t.Errorf("used = %v, want [true, false]", used)
	}
}

// --- isCovered ---

func TestIsCovered_Covered(t *testing.T) {
	effective := []rbac.ScopedRule{
		eff("", "pods", []string{"get", "list"}, "default"),
	}
	if !isCovered(obs("", "pods", "get", "default"), effective) {
		t.Error("expected covered")
	}
}

func TestIsCovered_NotCovered(t *testing.T) {
	effective := []rbac.ScopedRule{
		eff("", "pods", []string{"get"}, "default"),
	}
	if isCovered(obs("", "secrets", "get", "default"), effective) {
		t.Error("expected not covered")
	}
}

func TestIsCovered_NonResourceURL(t *testing.T) {
	effective := []rbac.ScopedRule{
		effNonResource("/metrics", []string{"get"}),
	}
	if !isCovered(obsNonResource("/metrics", "get"), effective) {
		t.Error("expected covered for non-resource URL")
	}
	if isCovered(obsNonResource("/healthz", "get"), effective) {
		t.Error("expected not covered for different URL")
	}
}

// --- matchesResourceRule: direct unit tests ---

func TestMatchesResourceRule_ExactMatch(t *testing.T) {
	o := obs("", "pods", "get", "default")
	e := eff("", "pods", []string{"get"}, "default")
	if !matchesResourceRule(o, e) {
		t.Error("exact match should return true")
	}
}

func TestMatchesResourceRule_ClusterWideCoversAnyNS(t *testing.T) {
	o := obs("", "pods", "get", "kube-system")
	e := eff("", "pods", []string{"get"}, "") // cluster-wide
	if !matchesResourceRule(o, e) {
		t.Error("cluster-wide rule should cover any namespace")
	}
}

func TestMatchesResourceRule_NamespaceScopedDoesNotCross(t *testing.T) {
	o := obs("", "pods", "get", "kube-system")
	e := eff("", "pods", []string{"get"}, "default")
	if matchesResourceRule(o, e) {
		t.Error("namespace-scoped rule should not cross namespaces")
	}
}

func TestMatchesResourceRule_ResourceNamesExcludes(t *testing.T) {
	o := obs("", "configmaps", "get", "default")
	e := effWithResourceNames("", "configmaps", []string{"get"}, []string{"my-config"}, "default")
	if matchesResourceRule(o, e) {
		t.Error("effective rule with ResourceNames should not cover general observed rule")
	}
}

func TestMatchesResourceRule_WildcardAPIGroup(t *testing.T) {
	o := obs("apps", "deployments", "get", "default")
	e := eff("*", "deployments", []string{"get"}, "default")
	if !matchesResourceRule(o, e) {
		t.Error("wildcard apiGroup should match any apiGroup")
	}
}

func TestMatchesResourceRule_WildcardResource(t *testing.T) {
	o := obs("", "pods", "get", "default")
	e := eff("", "*", []string{"get"}, "default")
	if !matchesResourceRule(o, e) {
		t.Error("wildcard resource should match any resource")
	}
}

func TestMatchesResourceRule_WildcardVerb(t *testing.T) {
	o := obs("", "pods", "delete", "default")
	e := eff("", "pods", []string{"*"}, "default")
	if !matchesResourceRule(o, e) {
		t.Error("wildcard verb should match any verb")
	}
}

func TestMatchesResourceRule_APIGroupMismatch(t *testing.T) {
	o := obs("apps", "deployments", "get", "default")
	e := eff("batch", "deployments", []string{"get"}, "default")
	if matchesResourceRule(o, e) {
		t.Error("mismatched apiGroup should return false")
	}
}

func TestMatchesResourceRule_VerbMismatch(t *testing.T) {
	o := obs("", "pods", "delete", "default")
	e := eff("", "pods", []string{"get", "list"}, "default")
	if matchesResourceRule(o, e) {
		t.Error("mismatched verb should return false")
	}
}

// --- matchesNonResourceURL: direct unit tests ---

func TestMatchesNonResourceURL_ExactMatch(t *testing.T) {
	o := obsNonResource("/metrics", "get")
	e := effNonResource("/metrics", []string{"get"})
	if !matchesNonResourceURL(o, e) {
		t.Error("exact match should return true")
	}
}

func TestMatchesNonResourceURL_URLMismatch(t *testing.T) {
	o := obsNonResource("/healthz", "get")
	e := effNonResource("/metrics", []string{"get"})
	if matchesNonResourceURL(o, e) {
		t.Error("URL mismatch should return false")
	}
}

func TestMatchesNonResourceURL_VerbMismatch(t *testing.T) {
	o := obsNonResource("/metrics", "post")
	e := effNonResource("/metrics", []string{"get"})
	if matchesNonResourceURL(o, e) {
		t.Error("verb mismatch should return false")
	}
}

func TestMatchesNonResourceURL_EffectiveHasNoURLs(t *testing.T) {
	o := obsNonResource("/metrics", "get")
	e := eff("", "pods", []string{"get"}, "default") // resource rule, not non-resource
	if matchesNonResourceURL(o, e) {
		t.Error("effective rule without NonResourceURLs should not match")
	}
}

func TestMatchesNonResourceURL_WildcardURL(t *testing.T) {
	o := obsNonResource("/metrics", "get")
	e := effNonResource("*", []string{"get"})
	if !matchesNonResourceURL(o, e) {
		t.Error("wildcard URL should match any URL")
	}
}

// --- markUsed: overlapping effective rules ---

func TestMarkUsed_OverlappingEffectiveRules(t *testing.T) {
	// One observed action should mark ALL matching effective rules, not just the first.
	effective := []rbac.ScopedRule{
		eff("", "pods", []string{"get", "list"}, "default"),
		eff("", "pods", []string{"get"}, ""),            // cluster-wide also covers default
		eff("", "*", []string{"get"}, "default"),        // wildcard resource also covers pods
		eff("", "services", []string{"get"}, "default"), // unrelated, should NOT be marked
	}
	used := make([]bool, 4)

	markUsed(obs("", "pods", "get", "default"), effective, used)
	if !used[0] {
		t.Error("effective[0] should be marked (exact match)")
	}
	if !used[1] {
		t.Error("effective[1] should be marked (cluster-wide covers default)")
	}
	if !used[2] {
		t.Error("effective[2] should be marked (wildcard resource covers pods)")
	}
	if used[3] {
		t.Error("effective[3] should NOT be marked (different resource)")
	}
}

// --- sliceCovers: edge cases ---

func TestSliceCovers_BothEmpty(t *testing.T) {
	if !sliceCovers(nil, nil) {
		t.Error("empty granted + empty required should return true")
	}
}

func TestSliceCovers_WildcardAmongOther(t *testing.T) {
	if !sliceCovers([]string{"get", "*", "list"}, []string{"delete", "patch"}) {
		t.Error("wildcard among other entries should cover everything")
	}
}

// --- Evaluate: boundary score values ---

func TestEvaluate_ScoreBoundary_80_Green(t *testing.T) {
	// 4 used out of 5 → 80 → Green
	observed := []audiciav1alpha1.ObservedRule{
		obs("", "pods", "get", "default"),
		obs("", "pods", "list", "default"),
		obs("", "pods", "watch", "default"),
		obs("", "pods", "create", "default"),
	}
	effective := []rbac.ScopedRule{
		eff("", "pods", []string{"get"}, "default"),
		eff("", "pods", []string{"list"}, "default"),
		eff("", "pods", []string{"watch"}, "default"),
		eff("", "pods", []string{"create"}, "default"),
		eff("", "secrets", []string{"get"}, "default"), // excess
	}

	report := Evaluate(observed, effective)
	if report == nil {
		t.Fatal("expected non-nil report")
	}
	if report.Score != 80 {
		t.Errorf("expected score 80 (boundary), got %d", report.Score)
	}
	if report.Severity != audiciav1alpha1.ComplianceSeverityGreen {
		t.Errorf("expected Green at score 80, got %s", report.Severity)
	}
}

func TestEvaluate_ScoreBoundary_50_Yellow(t *testing.T) {
	// 1 used out of 2 → 50 → Yellow
	observed := []audiciav1alpha1.ObservedRule{
		obs("", "pods", "get", "default"),
	}
	effective := []rbac.ScopedRule{
		eff("", "pods", []string{"get"}, "default"),
		eff("", "secrets", []string{"get"}, "default"),
	}

	report := Evaluate(observed, effective)
	if report == nil {
		t.Fatal("expected non-nil report")
	}
	if report.Score != 50 {
		t.Errorf("expected score 50 (boundary), got %d", report.Score)
	}
	if report.Severity != audiciav1alpha1.ComplianceSeverityYellow {
		t.Errorf("expected Yellow at score 50, got %s", report.Severity)
	}
}

// --- Evaluate: one effective rule covers multiple observed ---

func TestEvaluate_OneEffectiveCoversMultipleObserved(t *testing.T) {
	observed := []audiciav1alpha1.ObservedRule{
		obs("", "pods", "get", "default"),
		obs("", "pods", "list", "default"),
		obs("", "pods", "watch", "default"),
		obs("", "pods", "create", "default"),
		obs("", "pods", "delete", "default"),
	}
	effective := []rbac.ScopedRule{
		eff("", "pods", []string{"*"}, "default"),
	}

	report := Evaluate(observed, effective)
	if report == nil {
		t.Fatal("expected non-nil report")
	}
	if report.Score != 100 {
		t.Errorf("expected score 100, got %d", report.Score)
	}
	if report.UsedCount != 1 {
		t.Errorf("expected 1 used effective rule, got %d", report.UsedCount)
	}
	if report.UncoveredCount != 0 {
		t.Errorf("expected 0 uncovered, got %d", report.UncoveredCount)
	}
}
