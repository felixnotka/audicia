package rbac

import (
	"context"
	"testing"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	audiciav1alpha1 "github.com/felixnotka/audicia/operator/pkg/apis/audicia.io/v1alpha1"
)

func testScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(s)
	return s
}

func makeClusterRole(name string, rules []rbacv1.PolicyRule) *rbacv1.ClusterRole {
	return &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Rules:      rules,
	}
}

func makeRole(name, namespace string, rules []rbacv1.PolicyRule) *rbacv1.Role {
	return &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Rules:      rules,
	}
}

func makeCRB(name, crName string, subjects []rbacv1.Subject) *rbacv1.ClusterRoleBinding {
	return &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		RoleRef:    rbacv1.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: "ClusterRole", Name: crName},
		Subjects:   subjects,
	}
}

func makeRB(name, namespace, roleKind, roleName string, subjects []rbacv1.Subject) *rbacv1.RoleBinding {
	return &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		RoleRef:    rbacv1.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: roleKind, Name: roleName},
		Subjects:   subjects,
	}
}

var podReadRules = []rbacv1.PolicyRule{
	{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get", "list"}},
}

var secretReadRules = []rbacv1.PolicyRule{
	{APIGroups: []string{""}, Resources: []string{"secrets"}, Verbs: []string{"get"}},
}

// --- Tests ---

func TestEffectiveRules_SA_ClusterRoleBinding(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(
		makeClusterRole("reader", podReadRules),
		makeCRB("reader-binding", "reader", []rbacv1.Subject{
			{Kind: "ServiceAccount", Name: "backend", Namespace: "prod"},
		}),
	).Build()

	resolver := NewResolver(c)
	rules, err := resolver.EffectiveRules(context.Background(), audiciav1alpha1.Subject{
		Kind: audiciav1alpha1.SubjectKindServiceAccount, Name: "backend", Namespace: "prod",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 {
		t.Fatalf("got %d rules, want 1", len(rules))
	}
	if rules[0].Namespace != "" {
		t.Errorf("CRB should produce cluster-scoped rule (empty namespace), got %q", rules[0].Namespace)
	}
	if len(rules[0].Verbs) != 2 {
		t.Errorf("got %d verbs, want 2", len(rules[0].Verbs))
	}
}

func TestEffectiveRules_SA_RoleBinding_Role(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(
		makeRole("pod-reader", "prod", podReadRules),
		makeRB("pod-reader-binding", "prod", "Role", "pod-reader", []rbacv1.Subject{
			{Kind: "ServiceAccount", Name: "backend", Namespace: "prod"},
		}),
	).Build()

	resolver := NewResolver(c)
	rules, err := resolver.EffectiveRules(context.Background(), audiciav1alpha1.Subject{
		Kind: audiciav1alpha1.SubjectKindServiceAccount, Name: "backend", Namespace: "prod",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 {
		t.Fatalf("got %d rules, want 1", len(rules))
	}
	if rules[0].Namespace != "prod" {
		t.Errorf("RoleBinding should scope to namespace 'prod', got %q", rules[0].Namespace)
	}
}

func TestEffectiveRules_SA_RoleBinding_ClusterRole(t *testing.T) {
	// A RoleBinding that references a ClusterRole scopes it to the RB's namespace.
	c := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(
		makeClusterRole("reader", podReadRules),
		makeRB("reader-binding", "staging", "ClusterRole", "reader", []rbacv1.Subject{
			{Kind: "ServiceAccount", Name: "frontend", Namespace: "staging"},
		}),
	).Build()

	resolver := NewResolver(c)
	rules, err := resolver.EffectiveRules(context.Background(), audiciav1alpha1.Subject{
		Kind: audiciav1alpha1.SubjectKindServiceAccount, Name: "frontend", Namespace: "staging",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 {
		t.Fatalf("got %d rules, want 1", len(rules))
	}
	if rules[0].Namespace != "staging" {
		t.Errorf("RB referencing ClusterRole should scope to RB namespace 'staging', got %q", rules[0].Namespace)
	}
}

func TestEffectiveRules_UserMatch(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(
		makeClusterRole("admin", podReadRules),
		makeCRB("admin-alice", "admin", []rbacv1.Subject{
			{Kind: "User", Name: "alice@example.com", APIGroup: "rbac.authorization.k8s.io"},
		}),
	).Build()

	resolver := NewResolver(c)
	rules, err := resolver.EffectiveRules(context.Background(), audiciav1alpha1.Subject{
		Kind: audiciav1alpha1.SubjectKindUser, Name: "alice@example.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 {
		t.Fatalf("got %d rules, want 1", len(rules))
	}
}

func TestEffectiveRules_GroupMatch(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(
		makeClusterRole("viewer", podReadRules),
		makeCRB("viewer-devs", "viewer", []rbacv1.Subject{
			{Kind: "Group", Name: "developers", APIGroup: "rbac.authorization.k8s.io"},
		}),
	).Build()

	resolver := NewResolver(c)
	rules, err := resolver.EffectiveRules(context.Background(), audiciav1alpha1.Subject{
		Kind: audiciav1alpha1.SubjectKindGroup, Name: "developers",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 {
		t.Fatalf("got %d rules, want 1", len(rules))
	}
}

func TestEffectiveRules_NoMatchingBindings(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(
		makeClusterRole("admin", podReadRules),
		makeCRB("admin-binding", "admin", []rbacv1.Subject{
			{Kind: "ServiceAccount", Name: "other-sa", Namespace: "other-ns"},
		}),
	).Build()

	resolver := NewResolver(c)
	rules, err := resolver.EffectiveRules(context.Background(), audiciav1alpha1.Subject{
		Kind: audiciav1alpha1.SubjectKindServiceAccount, Name: "backend", Namespace: "prod",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 0 {
		t.Fatalf("got %d rules, want 0 (no matching bindings)", len(rules))
	}
}

func TestEffectiveRules_DeletedRole(t *testing.T) {
	// Binding exists but the referenced ClusterRole is missing.
	c := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(
		makeCRB("orphan-binding", "deleted-role", []rbacv1.Subject{
			{Kind: "ServiceAccount", Name: "backend", Namespace: "prod"},
		}),
	).Build()

	resolver := NewResolver(c)
	rules, err := resolver.EffectiveRules(context.Background(), audiciav1alpha1.Subject{
		Kind: audiciav1alpha1.SubjectKindServiceAccount, Name: "backend", Namespace: "prod",
	})
	if err != nil {
		t.Fatal("should not error on deleted role, got:", err)
	}
	if len(rules) != 0 {
		t.Fatalf("got %d rules, want 0 (deleted role skipped)", len(rules))
	}
}

func TestEffectiveRules_MultipleBindings(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(
		makeClusterRole("reader", podReadRules),
		makeRole("secret-reader", "prod", secretReadRules),
		makeCRB("reader-binding", "reader", []rbacv1.Subject{
			{Kind: "ServiceAccount", Name: "backend", Namespace: "prod"},
		}),
		makeRB("secret-binding", "prod", "Role", "secret-reader", []rbacv1.Subject{
			{Kind: "ServiceAccount", Name: "backend", Namespace: "prod"},
		}),
	).Build()

	resolver := NewResolver(c)
	rules, err := resolver.EffectiveRules(context.Background(), audiciav1alpha1.Subject{
		Kind: audiciav1alpha1.SubjectKindServiceAccount, Name: "backend", Namespace: "prod",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 2 {
		t.Fatalf("got %d rules, want 2 (one from CRB, one from RB)", len(rules))
	}

	// Verify we got both cluster-scoped and namespace-scoped rules.
	hasCluster := false
	hasNamespaced := false
	for _, r := range rules {
		if r.Namespace == "" {
			hasCluster = true
		}
		if r.Namespace == "prod" {
			hasNamespaced = true
		}
	}
	if !hasCluster {
		t.Error("expected a cluster-scoped rule from CRB")
	}
	if !hasNamespaced {
		t.Error("expected a namespace-scoped rule from RB")
	}
}

func TestEffectiveRules_SA_WrongNamespace(t *testing.T) {
	// SA match requires both name AND namespace.
	c := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(
		makeClusterRole("reader", podReadRules),
		makeCRB("reader-binding", "reader", []rbacv1.Subject{
			{Kind: "ServiceAccount", Name: "backend", Namespace: "prod"},
		}),
	).Build()

	resolver := NewResolver(c)
	rules, err := resolver.EffectiveRules(context.Background(), audiciav1alpha1.Subject{
		Kind: audiciav1alpha1.SubjectKindServiceAccount, Name: "backend", Namespace: "staging",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 0 {
		t.Fatalf("got %d rules, want 0 (SA namespace mismatch)", len(rules))
	}
}

// --- matchesSubject: direct unit tests ---

func TestMatchesSubject_SA_ExactMatch(t *testing.T) {
	subjects := []rbacv1.Subject{
		{Kind: "ServiceAccount", Name: "backend", Namespace: "prod"},
	}
	target := audiciav1alpha1.Subject{
		Kind: audiciav1alpha1.SubjectKindServiceAccount, Name: "backend", Namespace: "prod",
	}
	if !matchesSubject(subjects, target) {
		t.Error("exact SA match should return true")
	}
}

func TestMatchesSubject_SA_NameMismatch(t *testing.T) {
	subjects := []rbacv1.Subject{
		{Kind: "ServiceAccount", Name: "frontend", Namespace: "prod"},
	}
	target := audiciav1alpha1.Subject{
		Kind: audiciav1alpha1.SubjectKindServiceAccount, Name: "backend", Namespace: "prod",
	}
	if matchesSubject(subjects, target) {
		t.Error("SA name mismatch should return false")
	}
}

func TestMatchesSubject_SA_NamespaceMismatch(t *testing.T) {
	subjects := []rbacv1.Subject{
		{Kind: "ServiceAccount", Name: "backend", Namespace: "staging"},
	}
	target := audiciav1alpha1.Subject{
		Kind: audiciav1alpha1.SubjectKindServiceAccount, Name: "backend", Namespace: "prod",
	}
	if matchesSubject(subjects, target) {
		t.Error("SA namespace mismatch should return false")
	}
}

func TestMatchesSubject_User_Match(t *testing.T) {
	subjects := []rbacv1.Subject{
		{Kind: "User", Name: "alice@example.com", APIGroup: "rbac.authorization.k8s.io"},
	}
	target := audiciav1alpha1.Subject{
		Kind: audiciav1alpha1.SubjectKindUser, Name: "alice@example.com",
	}
	if !matchesSubject(subjects, target) {
		t.Error("user match should return true")
	}
}

func TestMatchesSubject_User_NameMismatch(t *testing.T) {
	subjects := []rbacv1.Subject{
		{Kind: "User", Name: "bob@example.com", APIGroup: "rbac.authorization.k8s.io"},
	}
	target := audiciav1alpha1.Subject{
		Kind: audiciav1alpha1.SubjectKindUser, Name: "alice@example.com",
	}
	if matchesSubject(subjects, target) {
		t.Error("user name mismatch should return false")
	}
}

func TestMatchesSubject_Group_Match(t *testing.T) {
	subjects := []rbacv1.Subject{
		{Kind: "Group", Name: "developers", APIGroup: "rbac.authorization.k8s.io"},
	}
	target := audiciav1alpha1.Subject{
		Kind: audiciav1alpha1.SubjectKindGroup, Name: "developers",
	}
	if !matchesSubject(subjects, target) {
		t.Error("group match should return true")
	}
}

func TestMatchesSubject_Group_NameMismatch(t *testing.T) {
	subjects := []rbacv1.Subject{
		{Kind: "Group", Name: "admins", APIGroup: "rbac.authorization.k8s.io"},
	}
	target := audiciav1alpha1.Subject{
		Kind: audiciav1alpha1.SubjectKindGroup, Name: "developers",
	}
	if matchesSubject(subjects, target) {
		t.Error("group name mismatch should return false")
	}
}

func TestMatchesSubject_KindMismatch(t *testing.T) {
	// User subject in binding should not match SA target.
	subjects := []rbacv1.Subject{
		{Kind: "User", Name: "backend", APIGroup: "rbac.authorization.k8s.io"},
	}
	target := audiciav1alpha1.Subject{
		Kind: audiciav1alpha1.SubjectKindServiceAccount, Name: "backend", Namespace: "prod",
	}
	if matchesSubject(subjects, target) {
		t.Error("kind mismatch (User binding vs SA target) should return false")
	}
}

func TestMatchesSubject_EmptySubjectList(t *testing.T) {
	target := audiciav1alpha1.Subject{
		Kind: audiciav1alpha1.SubjectKindUser, Name: "alice",
	}
	if matchesSubject(nil, target) {
		t.Error("empty subject list should return false")
	}
}

func TestMatchesSubject_MultipleSubjects_SecondMatches(t *testing.T) {
	subjects := []rbacv1.Subject{
		{Kind: "ServiceAccount", Name: "other", Namespace: "other"},
		{Kind: "User", Name: "alice"},
		{Kind: "ServiceAccount", Name: "backend", Namespace: "prod"},
	}
	target := audiciav1alpha1.Subject{
		Kind: audiciav1alpha1.SubjectKindServiceAccount, Name: "backend", Namespace: "prod",
	}
	if !matchesSubject(subjects, target) {
		t.Error("should match when target is in the list (not first)")
	}
}

// --- EffectiveRules: RoleBinding to deleted Role ---

func TestEffectiveRules_RoleBinding_DeletedRole(t *testing.T) {
	// RoleBinding references a Role that doesn't exist.
	c := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(
		makeRB("orphan-rb", "prod", "Role", "deleted-role", []rbacv1.Subject{
			{Kind: "ServiceAccount", Name: "backend", Namespace: "prod"},
		}),
	).Build()

	resolver := NewResolver(c)
	rules, err := resolver.EffectiveRules(context.Background(), audiciav1alpha1.Subject{
		Kind: audiciav1alpha1.SubjectKindServiceAccount, Name: "backend", Namespace: "prod",
	})
	if err != nil {
		t.Fatal("should not error on deleted role, got:", err)
	}
	if len(rules) != 0 {
		t.Fatalf("got %d rules, want 0 (deleted role skipped)", len(rules))
	}
}

// --- EffectiveRules: multiple rules in one Role ---

func TestEffectiveRules_MultipleRulesInRole(t *testing.T) {
	multiRules := []rbacv1.PolicyRule{
		{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}},
		{APIGroups: []string{""}, Resources: []string{"services"}, Verbs: []string{"list"}},
		{APIGroups: []string{"apps"}, Resources: []string{"deployments"}, Verbs: []string{"get", "list"}},
	}
	c := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(
		makeClusterRole("multi-reader", multiRules),
		makeCRB("multi-binding", "multi-reader", []rbacv1.Subject{
			{Kind: "ServiceAccount", Name: "backend", Namespace: "prod"},
		}),
	).Build()

	resolver := NewResolver(c)
	rules, err := resolver.EffectiveRules(context.Background(), audiciav1alpha1.Subject{
		Kind: audiciav1alpha1.SubjectKindServiceAccount, Name: "backend", Namespace: "prod",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 3 {
		t.Fatalf("got %d rules, want 3 (all PolicyRules from ClusterRole)", len(rules))
	}
}
