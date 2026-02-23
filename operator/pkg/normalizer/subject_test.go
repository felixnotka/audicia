package normalizer

import (
	"testing"

	audiciav1alpha1 "github.com/felixnotka/audicia/operator/pkg/apis/audicia.io/v1alpha1"
)

func TestNormalizeSubject_ServiceAccount(t *testing.T) {
	subject, include := NormalizeSubject("system:serviceaccount:prod:backend", true)
	if !include {
		t.Fatal("ServiceAccount should always be included")
	}
	if subject.Kind != audiciav1alpha1.SubjectKindServiceAccount {
		t.Errorf("Kind = %q, want ServiceAccount", subject.Kind)
	}
	if subject.Namespace != "prod" {
		t.Errorf("Namespace = %q, want prod", subject.Namespace)
	}
	if subject.Name != "backend" {
		t.Errorf("Name = %q, want backend", subject.Name)
	}
}

func TestNormalizeSubject_ServiceAccountAlwaysIncluded(t *testing.T) {
	// SAs are always included, even when ignoreSystemUsers is true.
	subject, include := NormalizeSubject("system:serviceaccount:kube-system:coredns", true)
	if !include {
		t.Fatal("ServiceAccount should be included even with ignoreSystemUsers=true")
	}
	if subject.Kind != audiciav1alpha1.SubjectKindServiceAccount {
		t.Errorf("Kind = %q, want ServiceAccount", subject.Kind)
	}
	if subject.Namespace != "kube-system" {
		t.Errorf("Namespace = %q, want kube-system", subject.Namespace)
	}
	if subject.Name != "coredns" {
		t.Errorf("Name = %q, want coredns", subject.Name)
	}
}

func TestNormalizeSubject_ServiceAccountWithColonsInName(t *testing.T) {
	// SplitN with limit=2 should keep everything after the second colon as the name.
	subject, include := NormalizeSubject("system:serviceaccount:ns:name:with:colons", true)
	if !include {
		t.Fatal("should be included")
	}
	if subject.Namespace != "ns" {
		t.Errorf("Namespace = %q, want ns", subject.Namespace)
	}
	if subject.Name != "name:with:colons" {
		t.Errorf("Name = %q, want name:with:colons", subject.Name)
	}
}

func TestNormalizeSubject_SystemUserExcluded(t *testing.T) {
	tests := []string{
		"system:kube-scheduler",
		"system:kube-controller-manager",
		"system:apiserver",
		"system:node:worker-1",
	}
	for _, username := range tests {
		_, include := NormalizeSubject(username, true)
		if include {
			t.Errorf("NormalizeSubject(%q, true) should exclude system user", username)
		}
	}
}

func TestNormalizeSubject_SystemUserIncludedWhenNotIgnored(t *testing.T) {
	subject, include := NormalizeSubject("system:kube-scheduler", false)
	if !include {
		t.Fatal("system user should be included when ignoreSystemUsers=false")
	}
	if subject.Kind != audiciav1alpha1.SubjectKindUser {
		t.Errorf("Kind = %q, want User", subject.Kind)
	}
	if subject.Name != "system:kube-scheduler" {
		t.Errorf("Name = %q, want system:kube-scheduler", subject.Name)
	}
}

func TestNormalizeSubject_RegularUser(t *testing.T) {
	subject, include := NormalizeSubject("alice@example.com", true)
	if !include {
		t.Fatal("regular user should be included")
	}
	if subject.Kind != audiciav1alpha1.SubjectKindUser {
		t.Errorf("Kind = %q, want User", subject.Kind)
	}
	if subject.Name != "alice@example.com" {
		t.Errorf("Name = %q, want alice@example.com", subject.Name)
	}
	if subject.Namespace != "" {
		t.Errorf("Namespace = %q, want empty for User", subject.Namespace)
	}
}

func TestNormalizeSubject_RegularUserWithSystemPrefix(t *testing.T) {
	// A non-system user whose name happens to not start with "system:".
	subject, include := NormalizeSubject("oidc:alice", true)
	if !include {
		t.Fatal("non-system user should be included")
	}
	if subject.Kind != audiciav1alpha1.SubjectKindUser {
		t.Errorf("Kind = %q, want User", subject.Kind)
	}
}

func TestNormalizeSubject_MalformedServiceAccount(t *testing.T) {
	// Only "system:serviceaccount:" with no further colons — falls through to system user logic.
	_, include := NormalizeSubject("system:serviceaccount:", true)
	// This has "system:" prefix but the SA parsing fails (SplitN returns 1 part).
	// Falls through to system user check — excluded because it starts with "system:".
	if include {
		t.Error("malformed SA with ignoreSystemUsers=true should be excluded")
	}
}

func TestNormalizeSubject_EmptyUsername(t *testing.T) {
	subject, include := NormalizeSubject("", true)
	if !include {
		t.Fatal("empty username should be included (not a system user)")
	}
	if subject.Kind != audiciav1alpha1.SubjectKindUser {
		t.Errorf("Kind = %q, want User", subject.Kind)
	}
	if subject.Name != "" {
		t.Errorf("Name = %q, want empty", subject.Name)
	}
}

func TestNormalizeSubject_MalformedSA_OnlyNamespace(t *testing.T) {
	// "system:serviceaccount:ns" — SplitN("ns", ":", 2) returns ["ns"],
	// len(parts)=1, falls through to system user check.
	_, include := NormalizeSubject("system:serviceaccount:ns", true)
	if include {
		t.Error("malformed SA with only namespace (no name) should be excluded as system user")
	}
}

func TestNormalizeSubject_MalformedSA_OnlyNamespace_IncludeWhenNotIgnored(t *testing.T) {
	subject, include := NormalizeSubject("system:serviceaccount:ns", false)
	if !include {
		t.Fatal("malformed SA should be included when ignoreSystemUsers=false")
	}
	// Falls through to regular user since SA parsing fails.
	if subject.Kind != audiciav1alpha1.SubjectKindUser {
		t.Errorf("Kind = %q, want User (fallthrough)", subject.Kind)
	}
}

func TestNormalizeSubject_ServiceAccount_EmptyNamespace(t *testing.T) {
	// "system:serviceaccount::myapp" — empty namespace, valid name.
	subject, include := NormalizeSubject("system:serviceaccount::myapp", true)
	if !include {
		t.Fatal("should be included (valid SA parse)")
	}
	if subject.Kind != audiciav1alpha1.SubjectKindServiceAccount {
		t.Errorf("Kind = %q, want ServiceAccount", subject.Kind)
	}
	if subject.Namespace != "" {
		t.Errorf("Namespace = %q, want empty", subject.Namespace)
	}
	if subject.Name != "myapp" {
		t.Errorf("Name = %q, want myapp", subject.Name)
	}
}
