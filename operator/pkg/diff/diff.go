// Package diff compares observed RBAC usage against effective permissions
// and produces a ComplianceReport.
package diff

import (
	"sort"
	"strings"
	"time"

	audiciav1alpha1 "github.com/felixnotka/audicia/operator/pkg/apis/audicia.io/v1alpha1"
	"github.com/felixnotka/audicia/operator/pkg/rbac"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// sensitiveResources are resource types considered high-risk when granted
// but not observed in use.
var sensitiveResources = map[string]bool{
	"secrets":                         true,
	"nodes":                           true,
	"clusterroles":                    true,
	"clusterrolebindings":             true,
	"roles":                           true,
	"rolebindings":                    true,
	"mutatingwebhookconfigurations":   true,
	"validatingwebhookconfigurations": true,
	"certificatesigningrequests":      true,
	"tokenreviews":                    true,
	"subjectaccessreviews":            true,
	"selfsubjectaccessreviews":        true,
	"selfsubjectrulesreviews":         true,
	"persistentvolumes":               true,
	"storageclasses":                  true,
	"customresourcedefinitions":       true,
	"serviceaccounts/token":           true,
}

// Evaluate compares observed usage against effective permissions and returns
// a ComplianceReport. The report captures how much of the granted RBAC is
// actually being used, identifies excess grants, and flags sensitive resources.
//
// Score formula: usedEffective / totalEffective * 100
//   - usedEffective = effective rules that were exercised by at least one observed action
//   - totalEffective = total number of effective rules granted to the subject
//
// Both numerator and denominator use the same unit (effective rules) to avoid
// score inflation when a single broad rule covers many observed actions.
//
// Severity thresholds:
//   - Green  (>= 80): tight permissions, little excess
//   - Yellow (>= 50): moderate overprivilege
//   - Red    (< 50):  significant overprivilege
func Evaluate(observed []audiciav1alpha1.ObservedRule, effective []rbac.ScopedRule) *audiciav1alpha1.ComplianceReport {
	if len(effective) == 0 && len(observed) == 0 {
		return &audiciav1alpha1.ComplianceReport{
			Score:             100,
			Severity:          audiciav1alpha1.ComplianceSeverityGreen,
			LastEvaluatedTime: metav1.NewTime(time.Now()),
		}
	}

	// No effective permissions but there are observed rules → no RBAC resolved,
	// likely the resolver could not find any bindings. Return nil to indicate
	// that compliance could not be evaluated.
	if len(effective) == 0 {
		return nil
	}

	// Track which effective rules are "used" (observed in practice).
	used := make([]bool, len(effective))
	var uncoveredCount int

	for _, obs := range observed {
		if !isCovered(obs, effective) {
			uncoveredCount++
		}
		markUsed(obs, effective, used)
	}

	// Count used and excess effective rules, detect sensitive excess.
	usedCount, excessCount, sensitiveExcess := classifyEffective(effective, used)

	// Calculate score: ratio of used effective rules to total effective rules.
	var score int32
	if len(effective) > 0 {
		score = int32(usedCount * 100 / len(effective))
	}

	severity := severityFromScore(score)

	return &audiciav1alpha1.ComplianceReport{
		Score:              score,
		Severity:           severity,
		UsedCount:          int32(usedCount),
		ExcessCount:        int32(excessCount),
		UncoveredCount:     int32(uncoveredCount),
		HasSensitiveExcess: len(sensitiveExcess) > 0,
		SensitiveExcess:    sensitiveExcess,
		LastEvaluatedTime:  metav1.NewTime(time.Now()),
	}
}

// classifyEffective partitions effective rules into used and excess, and
// detects sensitive resources among the excess grants.
func classifyEffective(effective []rbac.ScopedRule, used []bool) (usedCount, excessCount int, sensitiveExcess []string) {
	sensitiveSet := make(map[string]bool)

	for i, eff := range effective {
		if used[i] {
			usedCount++
			continue
		}
		excessCount++
		collectSensitive(eff.Resources, sensitiveSet, &sensitiveExcess)
	}

	sort.Strings(sensitiveExcess)
	return
}

// collectSensitive appends any sensitive or wildcard resources to the excess list.
func collectSensitive(resources []string, seen map[string]bool, out *[]string) {
	for _, res := range resources {
		resLower := strings.ToLower(res)
		if sensitiveResources[resLower] && !seen[resLower] {
			seen[resLower] = true
			*out = append(*out, resLower)
		}
		if res == "*" && !seen["*"] {
			seen["*"] = true
			*out = append(*out, "* (all resources)")
		}
	}
}

// severityFromScore maps a compliance score to a severity level.
func severityFromScore(score int32) audiciav1alpha1.ComplianceSeverity {
	switch {
	case score >= 80:
		return audiciav1alpha1.ComplianceSeverityGreen
	case score >= 50:
		return audiciav1alpha1.ComplianceSeverityYellow
	default:
		return audiciav1alpha1.ComplianceSeverityRed
	}
}

// isCovered checks whether an observed rule is authorized by at least one
// effective RBAC rule.
func isCovered(obs audiciav1alpha1.ObservedRule, effective []rbac.ScopedRule) bool {
	// Non-resource URLs are matched separately.
	if len(obs.NonResourceURLs) > 0 {
		for _, eff := range effective {
			if matchesNonResourceURL(obs, eff) {
				return true
			}
		}
		return false
	}

	for _, eff := range effective {
		if matchesResourceRule(obs, eff) {
			return true
		}
	}
	return false
}

// markUsed marks effective rules that are exercised by the given observed rule.
func markUsed(obs audiciav1alpha1.ObservedRule, effective []rbac.ScopedRule, used []bool) {
	if len(obs.NonResourceURLs) > 0 {
		for i, eff := range effective {
			if matchesNonResourceURL(obs, eff) {
				used[i] = true
			}
		}
		return
	}

	for i, eff := range effective {
		if matchesResourceRule(obs, eff) {
			used[i] = true
		}
	}
}

// matchesResourceRule checks whether a single effective rule covers the observed
// resource rule, respecting namespace scoping and wildcards.
//
// Conservative choices:
//   - ResourceNames-constrained rules are treated as NOT covering (audit events
//     don't capture which specific resource was accessed, only the resource type).
//   - Namespace-scoped rules only cover their own namespace; cluster-wide (ns="")
//     rules cover all namespaces.
func matchesResourceRule(obs audiciav1alpha1.ObservedRule, eff rbac.ScopedRule) bool {
	// Namespace check: cluster-wide rules (eff.Namespace=="") cover all namespaces.
	// Namespace-scoped rules only cover their own namespace.
	if eff.Namespace != "" && eff.Namespace != obs.Namespace {
		return false
	}

	// Effective rules with ResourceNames are more restrictive; we treat observed
	// rules (which don't specify resource names) as NOT covered by them.
	if len(eff.ResourceNames) > 0 {
		return false
	}

	// Check apiGroups, resources, and verbs.
	if !sliceCovers(eff.APIGroups, obs.APIGroups) {
		return false
	}
	if !sliceCovers(eff.Resources, obs.Resources) {
		return false
	}
	if !sliceCovers(eff.Verbs, obs.Verbs) {
		return false
	}
	return true
}

// matchesNonResourceURL checks whether an effective rule covers the observed
// non-resource URL rule.
func matchesNonResourceURL(obs audiciav1alpha1.ObservedRule, eff rbac.ScopedRule) bool {
	if len(eff.NonResourceURLs) == 0 {
		return false
	}
	if !sliceCovers(eff.NonResourceURLs, obs.NonResourceURLs) {
		return false
	}
	if !sliceCovers(eff.Verbs, obs.Verbs) {
		return false
	}
	return true
}

// sliceCovers returns true if every element in required is present in granted.
// A wildcard "*" in granted covers everything. Returns true when required is empty.
func sliceCovers(granted, required []string) bool {
	for _, g := range granted {
		if g == "*" {
			return true
		}
	}
	grantedSet := make(map[string]bool, len(granted))
	for _, g := range granted {
		grantedSet[g] = true
	}
	for _, r := range required {
		if !grantedSet[r] {
			return false
		}
	}
	return true
}
