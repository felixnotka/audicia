package aggregator

import (
	"sync"
	"testing"
	"time"

	audiciav1alpha1 "github.com/felixnotka/audicia/operator/pkg/apis/audicia.io/v1alpha1"
	"github.com/felixnotka/audicia/operator/pkg/normalizer"
)

func TestNew(t *testing.T) {
	agg := New()
	if agg == nil {
		t.Fatal("New() returned nil")
	}
	if len(agg.Rules()) != 0 {
		t.Errorf("new aggregator has %d rules, want 0", len(agg.Rules()))
	}
	if agg.EventsProcessed() != 0 {
		t.Errorf("new aggregator has %d events, want 0", agg.EventsProcessed())
	}
}

func TestAdd_SingleRule(t *testing.T) {
	agg := New()
	now := time.Now()
	agg.Add(normalizer.CanonicalRule{
		APIGroup:  "",
		Resource:  "pods",
		Verb:      "get",
		Namespace: "default",
	}, now)

	rules := agg.Rules()
	if len(rules) != 1 {
		t.Fatalf("got %d rules, want 1", len(rules))
	}

	r := rules[0]
	if r.Count != 1 {
		t.Errorf("Count = %d, want 1", r.Count)
	}
	if len(r.APIGroups) != 1 || r.APIGroups[0] != "" {
		t.Errorf("APIGroups = %v, want [\"\"]", r.APIGroups)
	}
	if len(r.Resources) != 1 || r.Resources[0] != "pods" {
		t.Errorf("Resources = %v, want [pods]", r.Resources)
	}
	if len(r.Verbs) != 1 || r.Verbs[0] != "get" {
		t.Errorf("Verbs = %v, want [get]", r.Verbs)
	}
	if r.Namespace != "default" {
		t.Errorf("Namespace = %q, want default", r.Namespace)
	}
	if agg.EventsProcessed() != 1 {
		t.Errorf("EventsProcessed = %d, want 1", agg.EventsProcessed())
	}
}

func TestAdd_Deduplication(t *testing.T) {
	agg := New()
	t1 := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2025, 1, 1, 11, 0, 0, 0, time.UTC)
	t3 := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	rule := normalizer.CanonicalRule{
		APIGroup:  "",
		Resource:  "pods",
		Verb:      "get",
		Namespace: "default",
	}

	agg.Add(rule, t1)
	agg.Add(rule, t2)
	agg.Add(rule, t3)

	rules := agg.Rules()
	if len(rules) != 1 {
		t.Fatalf("got %d rules, want 1 (deduplicated)", len(rules))
	}
	if rules[0].Count != 3 {
		t.Errorf("Count = %d, want 3", rules[0].Count)
	}
	if agg.EventsProcessed() != 3 {
		t.Errorf("EventsProcessed = %d, want 3", agg.EventsProcessed())
	}
}

func TestAdd_FirstSeenLastSeenTracking(t *testing.T) {
	agg := New()
	t1 := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	rule := normalizer.CanonicalRule{
		APIGroup:  "",
		Resource:  "pods",
		Verb:      "get",
		Namespace: "default",
	}

	agg.Add(rule, t1)
	agg.Add(rule, t2)

	rules := agg.Rules()
	if len(rules) != 1 {
		t.Fatalf("got %d rules, want 1", len(rules))
	}
	if !rules[0].FirstSeen.Time.Equal(t1) {
		t.Errorf("FirstSeen = %v, want %v", rules[0].FirstSeen.Time, t1)
	}
	if !rules[0].LastSeen.Time.Equal(t2) {
		t.Errorf("LastSeen = %v, want %v", rules[0].LastSeen.Time, t2)
	}
}

func TestAdd_DifferentVerbsAreSeparateRules(t *testing.T) {
	agg := New()
	now := time.Now()

	agg.Add(normalizer.CanonicalRule{Resource: "pods", Verb: "get", Namespace: "default"}, now)
	agg.Add(normalizer.CanonicalRule{Resource: "pods", Verb: "list", Namespace: "default"}, now)
	agg.Add(normalizer.CanonicalRule{Resource: "pods", Verb: "watch", Namespace: "default"}, now)

	rules := agg.Rules()
	if len(rules) != 3 {
		t.Errorf("got %d rules, want 3 (different verbs are separate)", len(rules))
	}
}

func TestAdd_DifferentNamespacesAreSeparateRules(t *testing.T) {
	agg := New()
	now := time.Now()

	agg.Add(normalizer.CanonicalRule{Resource: "pods", Verb: "get", Namespace: "prod"}, now)
	agg.Add(normalizer.CanonicalRule{Resource: "pods", Verb: "get", Namespace: "staging"}, now)

	rules := agg.Rules()
	if len(rules) != 2 {
		t.Errorf("got %d rules, want 2 (different namespaces)", len(rules))
	}
}

func TestAdd_NonResourceURL(t *testing.T) {
	agg := New()
	now := time.Now()

	agg.Add(normalizer.CanonicalRule{
		NonResourceURL: "/metrics",
		Verb:           "get",
	}, now)

	rules := agg.Rules()
	if len(rules) != 1 {
		t.Fatalf("got %d rules, want 1", len(rules))
	}
	if len(rules[0].NonResourceURLs) != 1 || rules[0].NonResourceURLs[0] != "/metrics" {
		t.Errorf("NonResourceURLs = %v, want [/metrics]", rules[0].NonResourceURLs)
	}
	if len(rules[0].APIGroups) != 0 {
		t.Errorf("APIGroups = %v, want empty for non-resource URL", rules[0].APIGroups)
	}
	if len(rules[0].Resources) != 0 {
		t.Errorf("Resources = %v, want empty for non-resource URL", rules[0].Resources)
	}
}

func TestRules_DeterministicSort(t *testing.T) {
	agg := New()
	now := time.Now()

	// Add in reverse order.
	agg.Add(normalizer.CanonicalRule{APIGroup: "apps", Resource: "deployments", Verb: "list", Namespace: "prod"}, now)
	agg.Add(normalizer.CanonicalRule{APIGroup: "", Resource: "pods", Verb: "get", Namespace: "default"}, now)
	agg.Add(normalizer.CanonicalRule{APIGroup: "", Resource: "configmaps", Verb: "get", Namespace: "default"}, now)
	agg.Add(normalizer.CanonicalRule{APIGroup: "", Resource: "pods", Verb: "get", Namespace: "prod"}, now)

	rules := agg.Rules()
	if len(rules) != 4 {
		t.Fatalf("got %d rules, want 4", len(rules))
	}

	// Expected sort: Namespace first, then APIGroup, then Resource, then Verb.
	// default/""/configmaps/get, default/""/pods/get, prod/""/pods/get, prod/apps/deployments/list
	if rules[0].Resources[0] != "configmaps" || rules[0].Namespace != "default" {
		t.Errorf("rules[0]: got %s/%s, want default/configmaps", rules[0].Namespace, rules[0].Resources[0])
	}
	if rules[1].Resources[0] != "pods" || rules[1].Namespace != "default" {
		t.Errorf("rules[1]: got %s/%s, want default/pods", rules[1].Namespace, rules[1].Resources[0])
	}
	if rules[2].Resources[0] != "pods" || rules[2].Namespace != "prod" {
		t.Errorf("rules[2]: got %s/%s, want prod/pods", rules[2].Namespace, rules[2].Resources[0])
	}
	if rules[3].Resources[0] != "deployments" || rules[3].Namespace != "prod" {
		t.Errorf("rules[3]: got %s/%s, want prod/deployments", rules[3].Namespace, rules[3].Resources[0])
	}
}

func TestAdd_ConcurrentSafety(t *testing.T) {
	agg := New()
	now := time.Now()
	const goroutines = 10
	const addsPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < addsPerGoroutine; i++ {
				agg.Add(normalizer.CanonicalRule{
					Resource:  "pods",
					Verb:      "get",
					Namespace: "default",
				}, now)
			}
		}(g)
	}
	wg.Wait()

	if agg.EventsProcessed() != goroutines*addsPerGoroutine {
		t.Errorf("EventsProcessed = %d, want %d", agg.EventsProcessed(), goroutines*addsPerGoroutine)
	}
	rules := agg.Rules()
	if len(rules) != 1 {
		t.Errorf("got %d rules, want 1 (all same key)", len(rules))
	}
	if rules[0].Count != goroutines*addsPerGoroutine {
		t.Errorf("Count = %d, want %d", rules[0].Count, goroutines*addsPerGoroutine)
	}
}

func TestAdd_MixedResourceAndNonResource(t *testing.T) {
	agg := New()
	now := time.Now()

	agg.Add(normalizer.CanonicalRule{Resource: "pods", Verb: "get", Namespace: "default"}, now)
	agg.Add(normalizer.CanonicalRule{NonResourceURL: "/metrics", Verb: "get"}, now)
	agg.Add(normalizer.CanonicalRule{NonResourceURL: "/healthz", Verb: "get"}, now)

	rules := agg.Rules()
	if len(rules) != 3 {
		t.Errorf("got %d rules, want 3", len(rules))
	}
	if agg.EventsProcessed() != 3 {
		t.Errorf("EventsProcessed = %d, want 3", agg.EventsProcessed())
	}
}

// --- ruleIsLess ---

func TestRuleIsLess_ByNamespace(t *testing.T) {
	a := audiciav1alpha1.ObservedRule{Namespace: "aaa", APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}}
	b := audiciav1alpha1.ObservedRule{Namespace: "zzz", APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}}
	if !ruleIsLess(a, b) {
		t.Error("expected a < b by namespace")
	}
	if ruleIsLess(b, a) {
		t.Error("expected b > a by namespace")
	}
}

func TestRuleIsLess_ByAPIGroup(t *testing.T) {
	a := audiciav1alpha1.ObservedRule{Namespace: "ns", APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}}
	b := audiciav1alpha1.ObservedRule{Namespace: "ns", APIGroups: []string{"apps"}, Resources: []string{"pods"}, Verbs: []string{"get"}}
	if !ruleIsLess(a, b) {
		t.Error("expected a < b by apiGroup")
	}
}

func TestRuleIsLess_ByResource(t *testing.T) {
	a := audiciav1alpha1.ObservedRule{Namespace: "ns", APIGroups: []string{""}, Resources: []string{"configmaps"}, Verbs: []string{"get"}}
	b := audiciav1alpha1.ObservedRule{Namespace: "ns", APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}}
	if !ruleIsLess(a, b) {
		t.Error("expected a < b by resource")
	}
}

func TestRuleIsLess_ByVerb(t *testing.T) {
	a := audiciav1alpha1.ObservedRule{Namespace: "ns", APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}}
	b := audiciav1alpha1.ObservedRule{Namespace: "ns", APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"list"}}
	if !ruleIsLess(a, b) {
		t.Error("expected a < b by verb")
	}
}

func TestRuleIsLess_Equal(t *testing.T) {
	a := audiciav1alpha1.ObservedRule{Namespace: "ns", APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}}
	if ruleIsLess(a, a) {
		t.Error("equal rules should return false")
	}
}

// --- firstElem ---

func TestFirstElem_NonEmpty(t *testing.T) {
	if got := firstElem([]string{"a", "b"}); got != "a" {
		t.Errorf("got %q, want %q", got, "a")
	}
}

func TestFirstElem_Empty(t *testing.T) {
	if got := firstElem(nil); got != "" {
		t.Errorf("got %q, want empty string", got)
	}
}

func TestFirstElem_SingleElement(t *testing.T) {
	if got := firstElem([]string{"only"}); got != "only" {
		t.Errorf("got %q, want %q", got, "only")
	}
}

// --- Deduplication key correctness ---

func TestAdd_DifferentAPIGroupsAreSeparate(t *testing.T) {
	agg := New()
	now := time.Now()

	agg.Add(normalizer.CanonicalRule{APIGroup: "", Resource: "deployments", Verb: "get", Namespace: "default"}, now)
	agg.Add(normalizer.CanonicalRule{APIGroup: "apps", Resource: "deployments", Verb: "get", Namespace: "default"}, now)

	rules := agg.Rules()
	if len(rules) != 2 {
		t.Errorf("got %d rules, want 2 (different apiGroups are separate keys)", len(rules))
	}
}

func TestAdd_ResourceVsNonResourceAreSeparate(t *testing.T) {
	agg := New()
	now := time.Now()

	agg.Add(normalizer.CanonicalRule{Resource: "pods", Verb: "get", Namespace: "default"}, now)
	agg.Add(normalizer.CanonicalRule{NonResourceURL: "/metrics", Verb: "get"}, now)

	rules := agg.Rules()
	if len(rules) != 2 {
		t.Errorf("got %d rules, want 2 (resource and non-resource are separate)", len(rules))
	}
}

func TestAdd_LastSeen_UpdatesCorrectly(t *testing.T) {
	agg := New()
	t1 := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2025, 1, 1, 9, 0, 0, 0, time.UTC) // earlier than t1

	rule := normalizer.CanonicalRule{Resource: "pods", Verb: "get", Namespace: "default"}

	agg.Add(rule, t1)
	agg.Add(rule, t2)

	rules := agg.Rules()
	// LastSeen should be the MOST RECENT timestamp passed to Add, not the
	// chronologically latest — it always overwrites.
	if !rules[0].LastSeen.Time.Equal(t2) {
		t.Errorf("LastSeen = %v, want %v (always overwrites with latest Add call)", rules[0].LastSeen.Time, t2)
	}
}
