package strategy

import (
	"strings"
	"testing"
	"time"

	audiciav1alpha1 "github.com/felixnotka/audicia/operator/pkg/apis/audicia.io/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// --- helpers ---

func ts(t time.Time) metav1.Time { return metav1.NewTime(t) }

var t0 = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

func makeRule(apiGroup, resource, verb, namespace string) audiciav1alpha1.ObservedRule {
	return audiciav1alpha1.ObservedRule{
		APIGroups: []string{apiGroup},
		Resources: []string{resource},
		Verbs:     []string{verb},
		Namespace: namespace,
		FirstSeen: ts(t0),
		LastSeen:  ts(t0),
		Count:     1,
	}
}

func makeNonResourceRule(url, verb string) audiciav1alpha1.ObservedRule {
	return audiciav1alpha1.ObservedRule{
		NonResourceURLs: []string{url},
		Verbs:           []string{verb},
		FirstSeen:       ts(t0),
		LastSeen:        ts(t0),
		Count:           1,
	}
}

func defaultEngine() *Engine {
	return NewEngine(audiciav1alpha1.PolicyStrategy{})
}

func manifestsContain(manifests []string, substr string) bool {
	for _, m := range manifests {
		if strings.Contains(m, substr) {
			return true
		}
	}
	return false
}

func manifestsContainAll(manifests []string, substrs ...string) []string {
	var missing []string
	for _, s := range substrs {
		if !manifestsContain(manifests, s) {
			missing = append(missing, s)
		}
	}
	return missing
}

// --- sanitizeForName ---

func TestSanitizeForName_Basic(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"backend", "backend"},
		{"Backend", "backend"},
		{"alice@example.com", "alice-at-example-com"},
		{"system:kube-scheduler", "system-kube-scheduler"},
		{"pods/exec", "pods-exec"},
		{"my.dotted.name", "my-dotted-name"},
	}
	for _, tt := range tests {
		got := sanitizeForName(tt.input)
		if got != tt.want {
			t.Errorf("sanitizeForName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSanitizeForName_Truncation(t *testing.T) {
	long := strings.Repeat("a", 100)
	got := sanitizeForName(long)
	if len(got) > 50 {
		t.Errorf("length = %d, want <= 50", len(got))
	}
}

func TestSanitizeForName_TrailingDashTrimmed(t *testing.T) {
	// A name that ends with a special character after substitution.
	got := sanitizeForName("test.")
	if strings.HasSuffix(got, "-") {
		t.Errorf("sanitizeForName(%q) = %q, has trailing dash", "test.", got)
	}
}

// --- NewEngine defaults ---

func TestNewEngine_Defaults(t *testing.T) {
	e := NewEngine(audiciav1alpha1.PolicyStrategy{})
	if e.ScopeMode != audiciav1alpha1.ScopeModeNamespaceStrict {
		t.Errorf("ScopeMode = %q, want NamespaceStrict", e.ScopeMode)
	}
	if e.VerbMerge != audiciav1alpha1.VerbMergeSmart {
		t.Errorf("VerbMerge = %q, want Smart", e.VerbMerge)
	}
	if e.Wildcards != audiciav1alpha1.WildcardModeForbidden {
		t.Errorf("Wildcards = %q, want Forbidden", e.Wildcards)
	}
}

func TestNewEngine_ExplicitValues(t *testing.T) {
	e := NewEngine(audiciav1alpha1.PolicyStrategy{
		ScopeMode: audiciav1alpha1.ScopeModeClusterScopeAllowed,
		VerbMerge: audiciav1alpha1.VerbMergeExact,
		Wildcards: audiciav1alpha1.WildcardModeSafe,
	})
	if e.ScopeMode != audiciav1alpha1.ScopeModeClusterScopeAllowed {
		t.Errorf("ScopeMode = %q", e.ScopeMode)
	}
	if e.VerbMerge != audiciav1alpha1.VerbMergeExact {
		t.Errorf("VerbMerge = %q", e.VerbMerge)
	}
	if e.Wildcards != audiciav1alpha1.WildcardModeSafe {
		t.Errorf("Wildcards = %q", e.Wildcards)
	}
}

// --- GenerateManifests: empty input ---

func TestGenerateManifests_EmptyRules(t *testing.T) {
	e := defaultEngine()
	manifests, err := e.GenerateManifests(
		audiciav1alpha1.Subject{Kind: audiciav1alpha1.SubjectKindUser, Name: "alice"},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if manifests != nil {
		t.Errorf("expected nil for empty rules, got %d manifests", len(manifests))
	}
}

// --- ServiceAccount: single namespace ---

func TestGenerateManifests_SA_SingleNamespace(t *testing.T) {
	e := defaultEngine()
	subject := audiciav1alpha1.Subject{
		Kind: audiciav1alpha1.SubjectKindServiceAccount, Name: "backend", Namespace: "prod",
	}
	rules := []audiciav1alpha1.ObservedRule{
		makeRule("", "pods", "get", "prod"),
		makeRule("", "pods", "list", "prod"),
	}

	manifests, err := e.GenerateManifests(subject, rules)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifests) != 2 {
		t.Fatalf("got %d manifests, want 2 (Role + RoleBinding)", len(manifests))
	}

	if missing := manifestsContainAll(manifests, "kind: Role", "name: suggested-backend-role", "namespace: prod"); len(missing) > 0 {
		t.Errorf("missing in manifests: %v", missing)
	}
	if missing := manifestsContainAll(manifests, "kind: RoleBinding", "name: suggested-backend-binding"); len(missing) > 0 {
		t.Errorf("missing in manifests: %v", missing)
	}
}

// --- ServiceAccount: cross-namespace ---

func TestGenerateManifests_SA_CrossNamespace(t *testing.T) {
	e := defaultEngine()
	subject := audiciav1alpha1.Subject{
		Kind: audiciav1alpha1.SubjectKindServiceAccount, Name: "backend", Namespace: "prod",
	}
	rules := []audiciav1alpha1.ObservedRule{
		makeRule("", "pods", "get", "prod"),
		makeRule("", "configmaps", "get", "shared"),
	}

	manifests, err := e.GenerateManifests(subject, rules)
	if err != nil {
		t.Fatal(err)
	}

	// Should get 4 manifests: Role+Binding for prod, Role+Binding for shared.
	if len(manifests) != 4 {
		t.Fatalf("got %d manifests, want 4 (2 Role+Binding pairs)", len(manifests))
	}

	// Home namespace uses simple name.
	if !manifestsContain(manifests, "name: suggested-backend-role") {
		t.Error("missing suggested-backend-role for home namespace")
	}
	// Cross-namespace includes the target namespace in the name.
	if !manifestsContain(manifests, "name: suggested-backend-shared-role") {
		t.Error("missing suggested-backend-shared-role for cross-namespace")
	}
}

// --- ServiceAccount: non-resource URLs get ClusterRole ---

func TestGenerateManifests_SA_NonResourceURL(t *testing.T) {
	e := defaultEngine()
	subject := audiciav1alpha1.Subject{
		Kind: audiciav1alpha1.SubjectKindServiceAccount, Name: "monitoring", Namespace: "monitoring",
	}
	rules := []audiciav1alpha1.ObservedRule{
		makeRule("", "pods", "get", "monitoring"),
		makeNonResourceRule("/metrics", "get"),
	}

	manifests, err := e.GenerateManifests(subject, rules)
	if err != nil {
		t.Fatal(err)
	}

	// Should have: Role+Binding in monitoring, ClusterRole+ClusterRoleBinding for /metrics.
	if len(manifests) != 4 {
		t.Fatalf("got %d manifests, want 4", len(manifests))
	}
	if !manifestsContain(manifests, "kind: ClusterRole") {
		t.Error("expected ClusterRole for non-resource URL")
	}
	if !manifestsContain(manifests, "/metrics") {
		t.Error("expected /metrics in ClusterRole")
	}
}

// --- ServiceAccount: only non-resource URLs ---

func TestGenerateManifests_SA_OnlyNonResourceURLs(t *testing.T) {
	e := defaultEngine()
	subject := audiciav1alpha1.Subject{
		Kind: audiciav1alpha1.SubjectKindServiceAccount, Name: "scraper", Namespace: "monitoring",
	}
	rules := []audiciav1alpha1.ObservedRule{
		makeNonResourceRule("/metrics", "get"),
		makeNonResourceRule("/healthz", "get"),
	}

	manifests, err := e.GenerateManifests(subject, rules)
	if err != nil {
		t.Fatal(err)
	}

	if len(manifests) != 2 {
		t.Fatalf("got %d manifests, want 2 (ClusterRole + ClusterRoleBinding)", len(manifests))
	}
	if !manifestsContain(manifests, "kind: ClusterRole") {
		t.Error("expected ClusterRole")
	}
}

// --- User: NamespaceStrict, single namespace ---

func TestGenerateManifests_User_NamespaceStrict_SingleNS(t *testing.T) {
	e := defaultEngine()
	subject := audiciav1alpha1.Subject{Kind: audiciav1alpha1.SubjectKindUser, Name: "alice"}
	rules := []audiciav1alpha1.ObservedRule{
		makeRule("", "pods", "get", "default"),
	}

	manifests, err := e.GenerateManifests(subject, rules)
	if err != nil {
		t.Fatal(err)
	}

	if len(manifests) != 2 {
		t.Fatalf("got %d manifests, want 2", len(manifests))
	}
	if !manifestsContain(manifests, "kind: Role") {
		t.Error("expected Role in NamespaceStrict mode")
	}
	// Should NOT be a ClusterRole.
	if manifestsContain(manifests, "kind: ClusterRole") {
		t.Error("unexpected ClusterRole in NamespaceStrict mode with namespaced rules")
	}
}

// --- User: NamespaceStrict, cluster-scoped only ---

func TestGenerateManifests_User_NamespaceStrict_ClusterScopedOnly(t *testing.T) {
	e := defaultEngine()
	subject := audiciav1alpha1.Subject{Kind: audiciav1alpha1.SubjectKindUser, Name: "admin"}
	rules := []audiciav1alpha1.ObservedRule{
		makeRule("", "namespaces", "list", ""), // cluster-scoped, empty namespace
	}

	manifests, err := e.GenerateManifests(subject, rules)
	if err != nil {
		t.Fatal(err)
	}

	if !manifestsContain(manifests, "kind: ClusterRole") {
		t.Error("expected ClusterRole for cluster-scoped rules in NamespaceStrict")
	}
}

// --- User: NamespaceStrict, multi-namespace ---

func TestGenerateManifests_User_NamespaceStrict_MultiNS(t *testing.T) {
	e := defaultEngine()
	subject := audiciav1alpha1.Subject{Kind: audiciav1alpha1.SubjectKindUser, Name: "alice"}
	rules := []audiciav1alpha1.ObservedRule{
		makeRule("", "pods", "get", "prod"),
		makeRule("", "pods", "get", "staging"),
	}

	manifests, err := e.GenerateManifests(subject, rules)
	if err != nil {
		t.Fatal(err)
	}

	// Should get per-namespace Role+Binding pairs.
	if len(manifests) != 4 {
		t.Fatalf("got %d manifests, want 4", len(manifests))
	}
	if !manifestsContain(manifests, "namespace: prod") {
		t.Error("expected Role in prod namespace")
	}
	if !manifestsContain(manifests, "namespace: staging") {
		t.Error("expected Role in staging namespace")
	}
}

// --- User: ClusterScopeAllowed ---

func TestGenerateManifests_User_ClusterScopeAllowed(t *testing.T) {
	e := NewEngine(audiciav1alpha1.PolicyStrategy{
		ScopeMode: audiciav1alpha1.ScopeModeClusterScopeAllowed,
	})
	subject := audiciav1alpha1.Subject{Kind: audiciav1alpha1.SubjectKindUser, Name: "alice"}
	rules := []audiciav1alpha1.ObservedRule{
		makeRule("", "pods", "get", "prod"),
		makeRule("", "pods", "get", "staging"),
	}

	manifests, err := e.GenerateManifests(subject, rules)
	if err != nil {
		t.Fatal(err)
	}

	if len(manifests) != 2 {
		t.Fatalf("got %d manifests, want 2 (ClusterRole + ClusterRoleBinding)", len(manifests))
	}
	if !manifestsContain(manifests, "kind: ClusterRole") {
		t.Error("expected ClusterRole in ClusterScopeAllowed mode")
	}
	if !manifestsContain(manifests, "kind: ClusterRoleBinding") {
		t.Error("expected ClusterRoleBinding")
	}
}

// --- SA ignores ClusterScopeAllowed ---

func TestGenerateManifests_SA_IgnoresClusterScopeAllowed(t *testing.T) {
	e := NewEngine(audiciav1alpha1.PolicyStrategy{
		ScopeMode: audiciav1alpha1.ScopeModeClusterScopeAllowed,
	})
	subject := audiciav1alpha1.Subject{
		Kind: audiciav1alpha1.SubjectKindServiceAccount, Name: "backend", Namespace: "prod",
	}
	rules := []audiciav1alpha1.ObservedRule{
		makeRule("", "pods", "get", "prod"),
	}

	manifests, err := e.GenerateManifests(subject, rules)
	if err != nil {
		t.Fatal(err)
	}

	// SA should still get per-namespace Roles, not a ClusterRole.
	if manifestsContain(manifests, "kind: ClusterRole") {
		t.Error("SA should not get ClusterRole even in ClusterScopeAllowed mode")
	}
	if !manifestsContain(manifests, "kind: Role") {
		t.Error("SA should get Role")
	}
}

// --- VerbMerge: Smart ---

func TestGenerateManifests_VerbMerge_Smart(t *testing.T) {
	e := defaultEngine() // Smart by default.
	subject := audiciav1alpha1.Subject{
		Kind: audiciav1alpha1.SubjectKindServiceAccount, Name: "backend", Namespace: "prod",
	}
	rules := []audiciav1alpha1.ObservedRule{
		makeRule("", "pods", "get", "prod"),
		makeRule("", "pods", "list", "prod"),
		makeRule("", "pods", "watch", "prod"),
	}

	manifests, err := e.GenerateManifests(subject, rules)
	if err != nil {
		t.Fatal(err)
	}

	// After Smart merge, all 3 verbs should be in one PolicyRule.
	// Check that the Role has all 3 verbs.
	role := manifests[0] // First manifest is the Role.
	for _, verb := range []string{"get", "list", "watch"} {
		if !strings.Contains(role, verb) {
			t.Errorf("merged Role missing verb %q", verb)
		}
	}
}

// --- VerbMerge: Exact ---

func TestGenerateManifests_VerbMerge_Exact(t *testing.T) {
	e := NewEngine(audiciav1alpha1.PolicyStrategy{
		VerbMerge: audiciav1alpha1.VerbMergeExact,
	})
	subject := audiciav1alpha1.Subject{
		Kind: audiciav1alpha1.SubjectKindServiceAccount, Name: "backend", Namespace: "prod",
	}
	rules := []audiciav1alpha1.ObservedRule{
		makeRule("", "pods", "get", "prod"),
		makeRule("", "pods", "list", "prod"),
	}

	manifests, err := e.GenerateManifests(subject, rules)
	if err != nil {
		t.Fatal(err)
	}

	// In Exact mode the rules should NOT be merged — but the PolicyRule
	// deduplication in renderRole may still collapse them if they are
	// identical. Since get != list, we should see both verbs as separate
	// entries in the rules array.
	role := manifests[0]

	// Count how many "- apiGroups:" entries appear (each is a PolicyRule).
	ruleCount := strings.Count(role, "- apiGroups:")
	if ruleCount != 2 {
		t.Errorf("Exact mode: got %d PolicyRules, want 2 (one per verb)", ruleCount)
	}
}

// --- Wildcards: Forbidden ---

func TestGenerateManifests_Wildcards_Forbidden(t *testing.T) {
	e := defaultEngine() // Forbidden by default.
	subject := audiciav1alpha1.Subject{
		Kind: audiciav1alpha1.SubjectKindServiceAccount, Name: "admin-sa", Namespace: "admin",
	}

	// All 8 standard verbs for pods.
	allVerbs := []string{"get", "list", "watch", "create", "update", "patch", "delete", "deletecollection"}
	var rules []audiciav1alpha1.ObservedRule
	for _, v := range allVerbs {
		rules = append(rules, makeRule("", "pods", v, "admin"))
	}

	manifests, err := e.GenerateManifests(subject, rules)
	if err != nil {
		t.Fatal(err)
	}

	// Even with all 8 verbs, Forbidden mode should NOT emit "*".
	for _, m := range manifests {
		if strings.Contains(m, `- '*'`) || strings.Contains(m, `"*"`) {
			t.Error("Wildcards Forbidden: found wildcard verb in output")
		}
	}
}

// --- Wildcards: Safe ---

func TestGenerateManifests_Wildcards_Safe(t *testing.T) {
	e := NewEngine(audiciav1alpha1.PolicyStrategy{
		Wildcards: audiciav1alpha1.WildcardModeSafe,
	})
	subject := audiciav1alpha1.Subject{
		Kind: audiciav1alpha1.SubjectKindServiceAccount, Name: "admin-sa", Namespace: "admin",
	}

	allVerbs := []string{"get", "list", "watch", "create", "update", "patch", "delete", "deletecollection"}
	var rules []audiciav1alpha1.ObservedRule
	for _, v := range allVerbs {
		rules = append(rules, makeRule("", "pods", v, "admin"))
	}

	manifests, err := e.GenerateManifests(subject, rules)
	if err != nil {
		t.Fatal(err)
	}

	// With all 8 verbs, Safe mode should collapse to "*".
	role := manifests[0]
	if !strings.Contains(role, `'*'`) && !strings.Contains(role, `"*"`) && !strings.Contains(role, "- '*'") {
		// YAML marshalling may render it differently. Check for the wildcard
		// as the sole verb.
		if !strings.Contains(role, `- "*"`) && !strings.Contains(role, "- '*'") {
			t.Errorf("Wildcards Safe: expected wildcard verb in output.\nRole:\n%s", role)
		}
	}
}

// --- Wildcards: Safe does NOT apply to non-resource URLs ---

func TestGenerateManifests_Wildcards_Safe_SkipsNonResourceURLs(t *testing.T) {
	e := NewEngine(audiciav1alpha1.PolicyStrategy{
		Wildcards: audiciav1alpha1.WildcardModeSafe,
	})
	subject := audiciav1alpha1.Subject{
		Kind: audiciav1alpha1.SubjectKindServiceAccount, Name: "scraper", Namespace: "monitoring",
	}

	// Even if all verbs are present for a non-resource URL, it shouldn't collapse.
	allVerbs := []string{"get", "list", "watch", "create", "update", "patch", "delete", "deletecollection"}
	var rules []audiciav1alpha1.ObservedRule
	for _, v := range allVerbs {
		rules = append(rules, makeNonResourceRule("/metrics", v))
	}

	manifests, err := e.GenerateManifests(subject, rules)
	if err != nil {
		t.Fatal(err)
	}

	for _, m := range manifests {
		if strings.Contains(m, "nonResourceURLs") {
			// Non-resource URL rules should not have wildcard.
			if strings.Contains(m, `'*'`) || strings.Contains(m, `"*"`) {
				t.Error("Wildcards Safe should not apply to non-resource URLs")
			}
		}
	}
}

// --- Verb filtering: non-standard verbs are dropped ---

func TestGenerateManifests_NonStandardVerbsFiltered(t *testing.T) {
	e := defaultEngine()
	subject := audiciav1alpha1.Subject{
		Kind: audiciav1alpha1.SubjectKindServiceAccount, Name: "backend", Namespace: "prod",
	}
	rules := []audiciav1alpha1.ObservedRule{
		{
			APIGroups: []string{""},
			Resources: []string{"pods"},
			Verbs:     []string{"get", "proxy", "nonstandard"},
			Namespace: "prod",
			FirstSeen: ts(t0),
			LastSeen:  ts(t0),
			Count:     1,
		},
	}

	manifests, err := e.GenerateManifests(subject, rules)
	if err != nil {
		t.Fatal(err)
	}

	role := manifests[0]
	if strings.Contains(role, "proxy") {
		t.Error("non-standard verb 'proxy' should be filtered out")
	}
	if strings.Contains(role, "nonstandard") {
		t.Error("non-standard verb 'nonstandard' should be filtered out")
	}
	if !strings.Contains(role, "get") {
		t.Error("standard verb 'get' should be preserved")
	}
}

// --- All verbs non-standard: rule dropped entirely ---

func TestGenerateManifests_AllNonStandardVerbsDropsRule(t *testing.T) {
	e := defaultEngine()
	subject := audiciav1alpha1.Subject{
		Kind: audiciav1alpha1.SubjectKindServiceAccount, Name: "backend", Namespace: "prod",
	}
	rules := []audiciav1alpha1.ObservedRule{
		{
			APIGroups: []string{""},
			Resources: []string{"pods"},
			Verbs:     []string{"proxy"},
			Namespace: "prod",
			FirstSeen: ts(t0),
			LastSeen:  ts(t0),
			Count:     1,
		},
	}

	manifests, err := e.GenerateManifests(subject, rules)
	if err != nil {
		t.Fatal(err)
	}

	// All verbs filtered out → no rules → no manifests.
	if manifests != nil {
		t.Errorf("expected nil manifests when all verbs are non-standard, got %d", len(manifests))
	}
}

// --- PolicyRule deduplication ---

func TestGenerateManifests_PolicyRuleDeduplication(t *testing.T) {
	e := defaultEngine()
	subject := audiciav1alpha1.Subject{
		Kind: audiciav1alpha1.SubjectKindServiceAccount, Name: "backend", Namespace: "prod",
	}
	// Same (apiGroup, resource, verb) observed in two different namespaces.
	// When rendered into a single Role (e.g., after per-namespace grouping puts
	// both in the home namespace), they should be deduplicated.
	rules := []audiciav1alpha1.ObservedRule{
		makeRule("", "pods", "get", "prod"),
		makeRule("", "pods", "get", "prod"), // exact duplicate
	}

	manifests, err := e.GenerateManifests(subject, rules)
	if err != nil {
		t.Fatal(err)
	}

	role := manifests[0]
	count := strings.Count(role, "- apiGroups:")
	if count != 1 {
		t.Errorf("expected 1 PolicyRule after dedup, got %d", count)
	}
}

// --- Binding name follows convention ---

func TestGenerateManifests_BindingNameConvention(t *testing.T) {
	e := defaultEngine()
	subject := audiciav1alpha1.Subject{
		Kind: audiciav1alpha1.SubjectKindServiceAccount, Name: "backend", Namespace: "prod",
	}
	rules := []audiciav1alpha1.ObservedRule{
		makeRule("", "pods", "get", "prod"),
	}

	manifests, err := e.GenerateManifests(subject, rules)
	if err != nil {
		t.Fatal(err)
	}

	if !manifestsContain(manifests, "name: suggested-backend-binding") {
		t.Error("expected binding name: suggested-backend-binding")
	}
}

// --- RoleBinding references correct Role ---

func TestGenerateManifests_BindingRefsCorrectRole(t *testing.T) {
	e := defaultEngine()
	subject := audiciav1alpha1.Subject{
		Kind: audiciav1alpha1.SubjectKindServiceAccount, Name: "backend", Namespace: "prod",
	}
	rules := []audiciav1alpha1.ObservedRule{
		makeRule("", "pods", "get", "prod"),
	}

	manifests, err := e.GenerateManifests(subject, rules)
	if err != nil {
		t.Fatal(err)
	}

	// The binding should reference the role name.
	binding := manifests[1]
	if !strings.Contains(binding, "name: suggested-backend-role") {
		t.Errorf("binding should reference suggested-backend-role.\nBinding:\n%s", binding)
	}
}

// --- Subject in binding has correct SA namespace ---

func TestGenerateManifests_BindingSubjectHasSANamespace(t *testing.T) {
	e := defaultEngine()
	subject := audiciav1alpha1.Subject{
		Kind: audiciav1alpha1.SubjectKindServiceAccount, Name: "backend", Namespace: "prod",
	}
	rules := []audiciav1alpha1.ObservedRule{
		makeRule("", "configmaps", "get", "shared"),
	}

	manifests, err := e.GenerateManifests(subject, rules)
	if err != nil {
		t.Fatal(err)
	}

	// The binding's subject should include the SA's home namespace.
	found := false
	for _, m := range manifests {
		if strings.Contains(m, "kind: RoleBinding") || strings.Contains(m, "kind: ClusterRoleBinding") {
			if strings.Contains(m, "namespace: prod") {
				found = true
			}
		}
	}
	if !found {
		t.Error("binding subject should include SA namespace 'prod'")
	}
}

// --- User binding has apiGroup ---

func TestGenerateManifests_UserBindingHasAPIGroup(t *testing.T) {
	e := defaultEngine()
	subject := audiciav1alpha1.Subject{Kind: audiciav1alpha1.SubjectKindUser, Name: "alice"}
	rules := []audiciav1alpha1.ObservedRule{
		makeRule("", "pods", "get", "default"),
	}

	manifests, err := e.GenerateManifests(subject, rules)
	if err != nil {
		t.Fatal(err)
	}

	binding := manifests[1]
	if !strings.Contains(binding, "apiGroup: rbac.authorization.k8s.io") {
		t.Errorf("User binding should have apiGroup.\nBinding:\n%s", binding)
	}
}

// --- YAML is parseable ---

func TestGenerateManifests_YAMLIsParseable(t *testing.T) {
	e := defaultEngine()
	subject := audiciav1alpha1.Subject{
		Kind: audiciav1alpha1.SubjectKindServiceAccount, Name: "backend", Namespace: "prod",
	}
	rules := []audiciav1alpha1.ObservedRule{
		makeRule("", "pods", "get", "prod"),
		makeRule("apps", "deployments", "list", "prod"),
		makeNonResourceRule("/metrics", "get"),
	}

	manifests, err := e.GenerateManifests(subject, rules)
	if err != nil {
		t.Fatal(err)
	}

	for i, m := range manifests {
		if strings.TrimSpace(m) == "" {
			t.Errorf("manifest[%d] is empty", i)
		}
		// Basic structural check: should contain "apiVersion:" and "kind:".
		if !strings.Contains(m, "apiVersion:") {
			t.Errorf("manifest[%d] missing apiVersion", i)
		}
		if !strings.Contains(m, "kind:") {
			t.Errorf("manifest[%d] missing kind", i)
		}
	}
}

// --- SA with cluster-scoped rules (empty namespace) defaults to home namespace ---

// --- mergeKeyForRule ---

func TestMergeKeyForRule_ResourceRule(t *testing.T) {
	r := makeRule("apps", "deployments", "get", "prod")
	key := mergeKeyForRule(r)
	if key.APIGroup != "apps" || key.Resource != "deployments" || key.Namespace != "prod" || key.NonResourceURL != "" {
		t.Errorf("unexpected key: %+v", key)
	}
}

func TestMergeKeyForRule_NonResourceURL(t *testing.T) {
	r := makeNonResourceRule("/metrics", "get")
	key := mergeKeyForRule(r)
	if key.NonResourceURL != "/metrics" || key.APIGroup != "" || key.Resource != "" {
		t.Errorf("unexpected key: %+v", key)
	}
}

// --- newMergedRule ---

func TestNewMergedRule(t *testing.T) {
	r := audiciav1alpha1.ObservedRule{
		APIGroups: []string{""},
		Resources: []string{"pods"},
		Verbs:     []string{"get", "list"},
		Namespace: "default",
		FirstSeen: ts(t0),
		LastSeen:  ts(t0),
		Count:     5,
	}
	m := newMergedRule(r)
	if !m.verbs["get"] || !m.verbs["list"] {
		t.Errorf("verbs = %v, want get and list", m.verbs)
	}
	if m.rule.Count != 5 {
		t.Errorf("Count = %d, want 5", m.rule.Count)
	}
}

// --- mergeInto ---

func TestMergeInto(t *testing.T) {
	t1 := t0
	t2 := t0.Add(time.Hour)

	existing := newMergedRule(audiciav1alpha1.ObservedRule{
		Verbs:     []string{"get"},
		FirstSeen: ts(t1),
		LastSeen:  ts(t1),
		Count:     3,
	})

	incoming := audiciav1alpha1.ObservedRule{
		Verbs:     []string{"list", "watch"},
		FirstSeen: ts(t2),
		LastSeen:  ts(t2),
		Count:     2,
	}

	mergeInto(existing, incoming)

	if !existing.verbs["get"] || !existing.verbs["list"] || !existing.verbs["watch"] {
		t.Errorf("verbs = %v, want get/list/watch", existing.verbs)
	}
	if existing.rule.Count != 5 {
		t.Errorf("Count = %d, want 5", existing.rule.Count)
	}
	// FirstSeen should stay at the earlier time.
	if !existing.rule.FirstSeen.Time.Equal(t1) {
		t.Errorf("FirstSeen = %v, want %v", existing.rule.FirstSeen.Time, t1)
	}
	// LastSeen should advance to the later time.
	if !existing.rule.LastSeen.Time.Equal(t2) {
		t.Errorf("LastSeen = %v, want %v", existing.rule.LastSeen.Time, t2)
	}
}

// --- flattenMerged ---

func TestFlattenMerged(t *testing.T) {
	k1 := mergeKey{APIGroup: "", Resource: "pods", Namespace: "default"}
	k2 := mergeKey{APIGroup: "apps", Resource: "deployments", Namespace: "default"}

	groups := map[mergeKey]*mergedRule{
		k1: {
			rule:  audiciav1alpha1.ObservedRule{APIGroups: []string{""}, Resources: []string{"pods"}, Namespace: "default"},
			verbs: map[string]bool{"get": true, "list": true},
		},
		k2: {
			rule:  audiciav1alpha1.ObservedRule{APIGroups: []string{"apps"}, Resources: []string{"deployments"}, Namespace: "default"},
			verbs: map[string]bool{"create": true},
		},
	}
	order := []mergeKey{k1, k2}

	result := flattenMerged(groups, order)
	if len(result) != 2 {
		t.Fatalf("got %d rules, want 2", len(result))
	}
	// First rule should have sorted verbs.
	if result[0].Verbs[0] != "get" || result[0].Verbs[1] != "list" {
		t.Errorf("verbs = %v, want [get, list]", result[0].Verbs)
	}
	if result[1].Verbs[0] != "create" {
		t.Errorf("verbs = %v, want [create]", result[1].Verbs)
	}
}

// --- hasAllStandardVerbs ---

func TestHasAllStandardVerbs_Complete(t *testing.T) {
	all := []string{"get", "list", "watch", "create", "update", "patch", "delete", "deletecollection"}
	if !hasAllStandardVerbs(all) {
		t.Error("expected true for all standard verbs")
	}
}

func TestHasAllStandardVerbs_Incomplete(t *testing.T) {
	partial := []string{"get", "list", "watch"}
	if hasAllStandardVerbs(partial) {
		t.Error("expected false for partial verb set")
	}
}

func TestHasAllStandardVerbs_TooFew(t *testing.T) {
	if hasAllStandardVerbs([]string{"get"}) {
		t.Error("expected false when fewer than 8 verbs")
	}
}

func TestHasAllStandardVerbs_SupersetTrue(t *testing.T) {
	superset := []string{"get", "list", "watch", "create", "update", "patch", "delete", "deletecollection", "custom"}
	if !hasAllStandardVerbs(superset) {
		t.Error("expected true for superset of standard verbs")
	}
}

// --- groupByNamespace ---

func TestGroupByNamespace_Basic(t *testing.T) {
	rules := []audiciav1alpha1.ObservedRule{
		makeRule("", "pods", "get", "prod"),
		makeRule("", "pods", "get", "staging"),
		makeRule("", "configmaps", "get", "prod"),
	}
	grouped := groupByNamespace(rules, "prod")
	if len(grouped["prod"]) != 2 {
		t.Errorf("prod rules = %d, want 2", len(grouped["prod"]))
	}
	if len(grouped["staging"]) != 1 {
		t.Errorf("staging rules = %d, want 1", len(grouped["staging"]))
	}
}

func TestGroupByNamespace_EmptyNSDefaultsToHome(t *testing.T) {
	rules := []audiciav1alpha1.ObservedRule{
		makeRule("", "namespaces", "list", ""), // cluster-scoped resource
	}
	grouped := groupByNamespace(rules, "monitoring")
	if len(grouped["monitoring"]) != 1 {
		t.Errorf("expected empty-ns resource to default to home ns, got groups: %v", grouped)
	}
}

func TestGroupByNamespace_NonResourceURLKeepsEmptyNS(t *testing.T) {
	rules := []audiciav1alpha1.ObservedRule{
		makeNonResourceRule("/metrics", "get"),
	}
	grouped := groupByNamespace(rules, "monitoring")
	if len(grouped[""]) != 1 {
		t.Errorf("expected non-resource URL to stay in empty-ns group, got groups: %v", grouped)
	}
}

// --- roleKindForNamespace ---

func TestRoleKindForNamespace(t *testing.T) {
	if got := roleKindForNamespace(""); got != "ClusterRole" {
		t.Errorf("got %q, want ClusterRole", got)
	}
	if got := roleKindForNamespace("default"); got != "Role" {
		t.Errorf("got %q, want Role", got)
	}
}

func TestGenerateManifests_SA_ClusterScopedDefaultsToHomeNS(t *testing.T) {
	e := defaultEngine()
	subject := audiciav1alpha1.Subject{
		Kind: audiciav1alpha1.SubjectKindServiceAccount, Name: "watcher", Namespace: "monitoring",
	}
	// A cluster-scoped watch (empty namespace) for a resource rule
	// should be assigned to the SA's home namespace.
	rules := []audiciav1alpha1.ObservedRule{
		makeRule("", "namespaces", "list", ""), // cluster-scoped
	}

	manifests, err := e.GenerateManifests(subject, rules)
	if err != nil {
		t.Fatal(err)
	}

	// Since it's a resource rule (not a non-resource URL), it should be
	// assigned to the home namespace "monitoring" and become a Role.
	if !manifestsContain(manifests, "kind: Role") {
		t.Error("expected Role (cluster-scoped resource defaults to home namespace for SA)")
	}
	if !manifestsContain(manifests, "namespace: monitoring") {
		t.Error("expected namespace: monitoring")
	}
}

// --- filterVerbs: non-resource URLs pass through unchanged ---

func TestFilterVerbs_NonResourceURLPreserved(t *testing.T) {
	e := defaultEngine()
	rules := []audiciav1alpha1.ObservedRule{
		makeNonResourceRule("/metrics", "get"),
	}
	result := e.filterVerbs(rules)
	if len(result) != 1 {
		t.Fatalf("got %d rules, want 1", len(result))
	}
	if len(result[0].NonResourceURLs) != 1 || result[0].NonResourceURLs[0] != "/metrics" {
		t.Errorf("NonResourceURLs = %v, want [/metrics]", result[0].NonResourceURLs)
	}
	if len(result[0].Verbs) != 1 || result[0].Verbs[0] != "get" {
		t.Errorf("Verbs = %v, want [get]", result[0].Verbs)
	}
}

func TestFilterVerbs_NonStandardVerbOnNonResourceURL(t *testing.T) {
	e := defaultEngine()
	rules := []audiciav1alpha1.ObservedRule{
		{
			NonResourceURLs: []string{"/metrics"},
			Verbs:           []string{"get", "proxy"},
			FirstSeen:       ts(t0),
			LastSeen:        ts(t0),
			Count:           1,
		},
	}
	result := e.filterVerbs(rules)
	if len(result) != 1 {
		t.Fatalf("got %d rules, want 1 (non-resource URL with one valid verb)", len(result))
	}
	if len(result[0].Verbs) != 1 || result[0].Verbs[0] != "get" {
		t.Errorf("Verbs = %v, want [get]", result[0].Verbs)
	}
}

// --- mergeVerbs: Exact mode is no-op ---

func TestMergeVerbs_ExactMode_NoOp(t *testing.T) {
	e := NewEngine(audiciav1alpha1.PolicyStrategy{
		VerbMerge: audiciav1alpha1.VerbMergeExact,
	})
	rules := []audiciav1alpha1.ObservedRule{
		makeRule("", "pods", "get", "default"),
		makeRule("", "pods", "list", "default"),
	}
	result := e.mergeVerbs(rules)
	if len(result) != 2 {
		t.Errorf("Exact mode should not merge: got %d rules, want 2", len(result))
	}
}

// --- mergeVerbs: different resources stay separate ---

func TestMergeVerbs_DifferentResourcesNotMerged(t *testing.T) {
	e := defaultEngine()
	rules := []audiciav1alpha1.ObservedRule{
		makeRule("", "pods", "get", "default"),
		makeRule("", "configmaps", "get", "default"),
	}
	result := e.mergeVerbs(rules)
	if len(result) != 2 {
		t.Errorf("different resources should not merge: got %d rules, want 2", len(result))
	}
}

// --- mergeVerbs: different namespaces stay separate ---

func TestMergeVerbs_DifferentNamespacesNotMerged(t *testing.T) {
	e := defaultEngine()
	rules := []audiciav1alpha1.ObservedRule{
		makeRule("", "pods", "get", "default"),
		makeRule("", "pods", "list", "prod"),
	}
	result := e.mergeVerbs(rules)
	if len(result) != 2 {
		t.Errorf("different namespaces should not merge: got %d rules, want 2", len(result))
	}
}

// --- mergeVerbs: non-resource URLs merge by URL ---

func TestMergeVerbs_NonResourceURLs(t *testing.T) {
	e := defaultEngine()
	rules := []audiciav1alpha1.ObservedRule{
		makeNonResourceRule("/metrics", "get"),
		makeNonResourceRule("/metrics", "post"),
	}
	result := e.mergeVerbs(rules)
	if len(result) != 1 {
		t.Fatalf("same URL should merge: got %d rules, want 1", len(result))
	}
	if len(result[0].Verbs) != 2 {
		t.Errorf("merged rule should have 2 verbs, got %v", result[0].Verbs)
	}
}

// --- User: NamespaceStrict multi-NS with cluster-scoped rules ---

func TestGenerateManifests_User_NamespaceStrict_MultiNS_WithClusterRules(t *testing.T) {
	e := defaultEngine()
	subject := audiciav1alpha1.Subject{Kind: audiciav1alpha1.SubjectKindUser, Name: "alice"}
	rules := []audiciav1alpha1.ObservedRule{
		makeRule("", "pods", "get", "prod"),
		makeRule("", "pods", "get", "staging"),
		makeRule("", "namespaces", "list", ""), // cluster-scoped
	}

	manifests, err := e.GenerateManifests(subject, rules)
	if err != nil {
		t.Fatal(err)
	}

	// Should get 4 manifests: Role+Binding for prod, Role+Binding for staging.
	// Cluster rules are merged into each namespace's Role.
	if len(manifests) != 4 {
		t.Fatalf("got %d manifests, want 4", len(manifests))
	}

	// Both namespace Roles (not RoleBindings) should contain the cluster-scoped
	// "namespaces" resource. Use "kind: Role\n" to avoid matching RoleBindings
	// which also contain "kind: Role" inside their roleRef block.
	for _, m := range manifests {
		if strings.Contains(m, "kind: Role\n") && !strings.Contains(m, "kind: RoleBinding") {
			if !strings.Contains(m, "namespaces") {
				t.Errorf("namespace Role should contain cluster-scoped 'namespaces' resource.\nManifest:\n%s", m)
			}
		}
	}
}

// --- User: only cluster-scoped rules in multi-NS path ---

func TestGenerateManifests_User_NamespaceStrict_OnlyClusterRules(t *testing.T) {
	e := defaultEngine()
	subject := audiciav1alpha1.Subject{Kind: audiciav1alpha1.SubjectKindUser, Name: "admin"}
	rules := []audiciav1alpha1.ObservedRule{
		makeRule("", "namespaces", "list", ""),
		makeRule("", "nodes", "get", ""),
	}

	manifests, err := e.GenerateManifests(subject, rules)
	if err != nil {
		t.Fatal(err)
	}

	// All cluster-scoped, single group → ClusterRole + ClusterRoleBinding.
	if len(manifests) != 2 {
		t.Fatalf("got %d manifests, want 2", len(manifests))
	}
	if !manifestsContain(manifests, "kind: ClusterRole") {
		t.Error("expected ClusterRole for cluster-scoped only rules")
	}
}

// --- Group subject binding has apiGroup ---

func TestGenerateManifests_GroupBindingHasAPIGroup(t *testing.T) {
	e := defaultEngine()
	subject := audiciav1alpha1.Subject{Kind: audiciav1alpha1.SubjectKindGroup, Name: "developers"}
	rules := []audiciav1alpha1.ObservedRule{
		makeRule("", "pods", "get", "default"),
	}

	manifests, err := e.GenerateManifests(subject, rules)
	if err != nil {
		t.Fatal(err)
	}

	binding := manifests[1]
	if !strings.Contains(binding, "apiGroup: rbac.authorization.k8s.io") {
		t.Errorf("Group binding should have apiGroup.\nBinding:\n%s", binding)
	}
	if !strings.Contains(binding, "kind: Group") {
		t.Errorf("Group binding should have kind: Group.\nBinding:\n%s", binding)
	}
}

// --- renderRole: cross-namespace dedup in strategy ---

func TestRenderRole_CrossNamespaceDedup(t *testing.T) {
	e := defaultEngine()
	// Two rules with same (apiGroup, resource, verb) but different namespaces.
	// When rendered into a single Role, they should be deduplicated because
	// PolicyRule has no namespace field.
	rules := []audiciav1alpha1.ObservedRule{
		makeRule("", "pods", "get", "prod"),
		makeRule("", "pods", "get", "staging"),
	}

	yaml := e.renderRole("Role", "test-role", "prod", rules)
	count := strings.Count(yaml, "- apiGroups:")
	if count != 1 {
		t.Errorf("expected 1 PolicyRule after dedup, got %d.\nYAML:\n%s", count, yaml)
	}
}

// --- applyWildcards: Forbidden mode is no-op even with all verbs ---

func TestApplyWildcards_ForbiddenMode_NoOp(t *testing.T) {
	e := NewEngine(audiciav1alpha1.PolicyStrategy{
		Wildcards: audiciav1alpha1.WildcardModeForbidden,
	})
	allVerbs := []string{"get", "list", "watch", "create", "update", "patch", "delete", "deletecollection"}
	rules := []audiciav1alpha1.ObservedRule{
		{
			APIGroups: []string{""}, Resources: []string{"pods"},
			Verbs: allVerbs, Namespace: "default",
			FirstSeen: ts(t0), LastSeen: ts(t0), Count: 1,
		},
	}
	result := e.applyWildcards(rules)
	if len(result[0].Verbs) != 8 {
		t.Errorf("Forbidden mode should not collapse verbs: got %d verbs", len(result[0].Verbs))
	}
}

// --- applyWildcards: Safe mode with partial verbs does NOT wildcard ---

func TestApplyWildcards_SafeMode_PartialVerbsNoWildcard(t *testing.T) {
	e := NewEngine(audiciav1alpha1.PolicyStrategy{
		Wildcards: audiciav1alpha1.WildcardModeSafe,
	})
	rules := []audiciav1alpha1.ObservedRule{
		{
			APIGroups: []string{""}, Resources: []string{"pods"},
			Verbs: []string{"get", "list", "watch"}, Namespace: "default",
			FirstSeen: ts(t0), LastSeen: ts(t0), Count: 1,
		},
	}
	result := e.applyWildcards(rules)
	if len(result[0].Verbs) != 3 {
		t.Errorf("partial verb set should not be collapsed: got %v", result[0].Verbs)
	}
}
