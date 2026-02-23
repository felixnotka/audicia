package filter

import (
	"testing"

	audiciav1alpha1 "github.com/felixnotka/audicia/operator/pkg/apis/audicia.io/v1alpha1"
)

func TestNewChain_EmptyRules(t *testing.T) {
	chain, err := NewChain(nil)
	if err != nil {
		t.Fatalf("NewChain(nil) returned error: %v", err)
	}
	if chain == nil {
		t.Fatal("NewChain(nil) returned nil chain")
	}
}

func TestNewChain_InvalidUserRegex(t *testing.T) {
	rules := []audiciav1alpha1.Filter{
		{Action: audiciav1alpha1.FilterActionDeny, UserPattern: "["},
	}
	_, err := NewChain(rules)
	if err == nil {
		t.Fatal("expected error for invalid regex, got nil")
	}
}

func TestNewChain_InvalidNamespaceRegex(t *testing.T) {
	rules := []audiciav1alpha1.Filter{
		{Action: audiciav1alpha1.FilterActionDeny, NamespacePattern: "(unclosed"},
	}
	_, err := NewChain(rules)
	if err == nil {
		t.Fatal("expected error for invalid namespace regex, got nil")
	}
}

func TestAllow_EmptyChainAllowsEverything(t *testing.T) {
	chain, err := NewChain(nil)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		username  string
		namespace string
	}{
		{"alice", "default"},
		{"system:kube-scheduler", "kube-system"},
		{"system:serviceaccount:prod:backend", "prod"},
		{"", ""},
	}

	for _, tt := range tests {
		if !chain.Allow(tt.username, tt.namespace) {
			t.Errorf("empty chain denied (%q, %q), want allow", tt.username, tt.namespace)
		}
	}
}

func TestAllow_DenyByUserPattern(t *testing.T) {
	chain, err := NewChain([]audiciav1alpha1.Filter{
		{Action: audiciav1alpha1.FilterActionDeny, UserPattern: ".*argocd.*"},
	})
	if err != nil {
		t.Fatal(err)
	}

	if chain.Allow("system:serviceaccount:argocd:argocd-server", "argocd") {
		t.Error("expected deny for argocd user, got allow")
	}
	if !chain.Allow("system:serviceaccount:prod:backend", "prod") {
		t.Error("expected allow for non-argocd user, got deny")
	}
}

func TestAllow_AllowByUserPattern(t *testing.T) {
	chain, err := NewChain([]audiciav1alpha1.Filter{
		{Action: audiciav1alpha1.FilterActionAllow, UserPattern: "^admin$"},
	})
	if err != nil {
		t.Fatal(err)
	}

	if !chain.Allow("admin", "default") {
		t.Error("expected allow for admin, got deny")
	}
	// Non-matching falls through to default allow.
	if !chain.Allow("alice", "default") {
		t.Error("expected allow for alice (no match, default allow), got deny")
	}
}

func TestAllow_DenyByNamespacePattern(t *testing.T) {
	chain, err := NewChain([]audiciav1alpha1.Filter{
		{Action: audiciav1alpha1.FilterActionDeny, NamespacePattern: "^kube-system$"},
	})
	if err != nil {
		t.Fatal(err)
	}

	if chain.Allow("system:kube-scheduler", "kube-system") {
		t.Error("expected deny for kube-system namespace, got allow")
	}
	if !chain.Allow("alice", "default") {
		t.Error("expected allow for default namespace, got deny")
	}
}

func TestAllow_CombinedUserAndNamespace_ORLogic(t *testing.T) {
	// A filter with both patterns set matches if EITHER matches.
	chain, err := NewChain([]audiciav1alpha1.Filter{
		{
			Action:           audiciav1alpha1.FilterActionDeny,
			UserPattern:      "^alice$",
			NamespacePattern: "^kube-system$",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Matches user pattern only.
	if chain.Allow("alice", "default") {
		t.Error("expected deny (user match), got allow")
	}
	// Matches namespace pattern only.
	if chain.Allow("bob", "kube-system") {
		t.Error("expected deny (namespace match), got allow")
	}
	// Matches both.
	if chain.Allow("alice", "kube-system") {
		t.Error("expected deny (both match), got allow")
	}
	// Matches neither — falls through to default allow.
	if !chain.Allow("bob", "default") {
		t.Error("expected allow (no match), got deny")
	}
}

func TestAllow_FirstMatchWins(t *testing.T) {
	chain, err := NewChain([]audiciav1alpha1.Filter{
		// 1. Allow cert-manager specifically.
		{Action: audiciav1alpha1.FilterActionAllow, UserPattern: ".*cert-manager.*"},
		// 2. Deny everything else.
		{Action: audiciav1alpha1.FilterActionDeny, UserPattern: ".*"},
	})
	if err != nil {
		t.Fatal(err)
	}

	// cert-manager matches rule 1 (Allow) before rule 2 (Deny).
	if !chain.Allow("system:serviceaccount:cert-manager:cert-manager", "cert-manager") {
		t.Error("expected allow for cert-manager (first match), got deny")
	}
	// Everything else matches rule 2 (Deny).
	if chain.Allow("system:serviceaccount:prod:backend", "prod") {
		t.Error("expected deny for backend (second match), got allow")
	}
}

func TestAllow_DenyAllThenAllowSpecific(t *testing.T) {
	// Common recipe: deny all system users, allow one namespace, deny the rest.
	chain, err := NewChain([]audiciav1alpha1.Filter{
		{Action: audiciav1alpha1.FilterActionDeny, UserPattern: `^system:node:.*`},
		{Action: audiciav1alpha1.FilterActionDeny, UserPattern: `^system:kube-.*`},
		{Action: audiciav1alpha1.FilterActionAllow, NamespacePattern: `^production$`},
		{Action: audiciav1alpha1.FilterActionDeny, UserPattern: ".*"},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Node users denied by rule 1.
	if chain.Allow("system:node:worker-1", "kube-system") {
		t.Error("node user should be denied")
	}
	// kube-controller-manager denied by rule 2.
	if chain.Allow("system:kube-controller-manager", "kube-system") {
		t.Error("kube system user should be denied")
	}
	// Production namespace allowed by rule 3.
	if !chain.Allow("system:serviceaccount:production:myapp", "production") {
		t.Error("production SA should be allowed")
	}
	// Non-production caught by rule 4 (deny all).
	if chain.Allow("system:serviceaccount:staging:myapp", "staging") {
		t.Error("staging SA should be denied by catch-all")
	}
}

func TestAllow_OnlyNamespacePattern(t *testing.T) {
	// A filter with only a namespace pattern and no user pattern.
	chain, err := NewChain([]audiciav1alpha1.Filter{
		{Action: audiciav1alpha1.FilterActionAllow, NamespacePattern: "^prod-.*"},
		{Action: audiciav1alpha1.FilterActionDeny, UserPattern: ".*"},
	})
	if err != nil {
		t.Fatal(err)
	}

	if !chain.Allow("any-user", "prod-frontend") {
		t.Error("prod-frontend namespace should be allowed")
	}
	if chain.Allow("any-user", "staging") {
		t.Error("staging should be denied")
	}
}

func TestAllow_NoPatternNeverMatches(t *testing.T) {
	// A filter with neither pattern should never match.
	chain, err := NewChain([]audiciav1alpha1.Filter{
		{Action: audiciav1alpha1.FilterActionDeny},
	})
	if err != nil {
		t.Fatal(err)
	}

	if !chain.Allow("anyone", "anywhere") {
		t.Error("filter with no patterns should never match, expected default allow")
	}
}
