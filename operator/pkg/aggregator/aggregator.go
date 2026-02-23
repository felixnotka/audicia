package aggregator

import (
	"sort"
	"sync"
	"time"

	audiciav1alpha1 "github.com/felixnotka/audicia/operator/pkg/apis/audicia.io/v1alpha1"
	"github.com/felixnotka/audicia/operator/pkg/normalizer"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ruleKey is the deduplication key for observed rules.
type ruleKey struct {
	APIGroup       string
	Resource       string
	Verb           string
	NonResourceURL string
	Namespace      string
}

// Aggregator deduplicates and merges observed rules per subject.
type Aggregator struct {
	mu    sync.RWMutex
	rules map[ruleKey]*audiciav1alpha1.ObservedRule
	count int64
}

// New creates a new Aggregator.
func New() *Aggregator {
	return &Aggregator{
		rules: make(map[ruleKey]*audiciav1alpha1.ObservedRule),
	}
}

// Add records a canonical rule observation. For duplicate keys, Count is
// incremented and LastSeen is unconditionally overwritten with the given
// timestamp (callers are expected to supply events in chronological order).
func (a *Aggregator) Add(rule normalizer.CanonicalRule, timestamp time.Time) {
	key := ruleKey{
		APIGroup:       rule.APIGroup,
		Resource:       rule.Resource,
		Verb:           rule.Verb,
		NonResourceURL: rule.NonResourceURL,
		Namespace:      rule.Namespace,
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	a.count++
	now := metav1.NewTime(timestamp)

	if existing, ok := a.rules[key]; ok {
		existing.Count++
		existing.LastSeen = now
		return
	}

	observed := &audiciav1alpha1.ObservedRule{
		Verbs:     []string{rule.Verb},
		Namespace: rule.Namespace,
		FirstSeen: now,
		LastSeen:  now,
		Count:     1,
	}

	if rule.NonResourceURL != "" {
		observed.NonResourceURLs = []string{rule.NonResourceURL}
	} else {
		observed.APIGroups = []string{rule.APIGroup}
		observed.Resources = []string{rule.Resource}
	}

	a.rules[key] = observed
}

// Rules returns the current aggregated rules as a deterministically sorted slice.
// Sorting order: Namespace, APIGroup, Resource, Verb (with non-resource URLs sorted after resources).
func (a *Aggregator) Rules() []audiciav1alpha1.ObservedRule {
	a.mu.RLock()
	defer a.mu.RUnlock()

	result := make([]audiciav1alpha1.ObservedRule, 0, len(a.rules))
	for _, rule := range a.rules {
		result = append(result, *rule)
	}

	sort.Slice(result, func(i, j int) bool {
		return ruleIsLess(result[i], result[j])
	})

	return result
}

// ruleIsLess compares two ObservedRules for deterministic sorting.
// Order: Namespace, APIGroup, Resource, Verb.
func ruleIsLess(a, b audiciav1alpha1.ObservedRule) bool {
	if a.Namespace != b.Namespace {
		return a.Namespace < b.Namespace
	}
	if ga, gb := firstElem(a.APIGroups), firstElem(b.APIGroups); ga != gb {
		return ga < gb
	}
	if ra, rb := firstElem(a.Resources), firstElem(b.Resources); ra != rb {
		return ra < rb
	}
	return firstElem(a.Verbs) < firstElem(b.Verbs)
}

// firstElem returns the first element of a string slice, or "" if empty.
func firstElem(s []string) string {
	if len(s) > 0 {
		return s[0]
	}
	return ""
}

// EventsProcessed returns the total number of events aggregated.
func (a *Aggregator) EventsProcessed() int64 {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.count
}
