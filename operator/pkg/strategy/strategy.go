package strategy

import (
	"fmt"
	"sort"
	"strings"

	audiciav1alpha1 "github.com/felixnotka/audicia/operator/pkg/apis/audicia.io/v1alpha1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

const (
	rbacAPIVersion = "rbac.authorization.k8s.io/v1"
	rbacAPIGroup   = "rbac.authorization.k8s.io"
)

// allowedVerbs is the set of standard Kubernetes verbs that Audicia will emit.
var allowedVerbs = map[string]bool{
	"get":              true,
	"list":             true,
	"watch":            true,
	"create":           true,
	"update":           true,
	"patch":            true,
	"delete":           true,
	"deletecollection": true,
}

// Engine applies policy strategy knobs to shape the final RBAC output.
type Engine struct {
	ScopeMode audiciav1alpha1.ScopeMode
	VerbMerge audiciav1alpha1.VerbMerge
	Wildcards audiciav1alpha1.WildcardMode
}

// NewEngine creates a strategy engine from an AudiciaSource policy strategy.
func NewEngine(ps audiciav1alpha1.PolicyStrategy) *Engine {
	e := &Engine{
		ScopeMode: ps.ScopeMode,
		VerbMerge: ps.VerbMerge,
		Wildcards: ps.Wildcards,
	}

	// Apply defaults.
	if e.ScopeMode == "" {
		e.ScopeMode = audiciav1alpha1.ScopeModeNamespaceStrict
	}
	if e.VerbMerge == "" {
		e.VerbMerge = audiciav1alpha1.VerbMergeSmart
	}
	if e.Wildcards == "" {
		e.Wildcards = audiciav1alpha1.WildcardModeForbidden
	}

	return e
}

// GenerateManifests produces rendered RBAC YAML from observed rules and subject.
func (e *Engine) GenerateManifests(subject audiciav1alpha1.Subject, rules []audiciav1alpha1.ObservedRule) ([]string, error) {
	if len(rules) == 0 {
		return nil, nil
	}

	// Filter to allowed verbs only.
	filteredRules := e.filterVerbs(rules)

	// Merge verbs for same resource when in Smart mode.
	filteredRules = e.mergeVerbs(filteredRules)

	// Collapse to wildcard when all verbs observed in Safe mode.
	filteredRules = e.applyWildcards(filteredRules)

	// ServiceAccounts: group rules by namespace and generate per-namespace
	// Role+RoleBinding pairs. A SA in namespace X may access resources in
	// namespaces Y and Z, so we need a Role in each target namespace.
	if subject.Kind == audiciav1alpha1.SubjectKindServiceAccount {
		return e.generatePerNamespace(subject, filteredRules), nil
	}

	// ClusterScopeAllowed mode: emit one ClusterRole for everything.
	if e.ScopeMode == audiciav1alpha1.ScopeModeClusterScopeAllowed {
		return e.generateSingleScope("ClusterRole", "", subject, filteredRules), nil
	}

	// NamespaceStrict mode for Users/Groups: group rules by namespace and generate
	// one Role+RoleBinding per namespace. Cluster-scoped rules (empty namespace,
	// non-resource URLs) get a ClusterRole only if no namespaced rules exist;
	// otherwise they are merged into each namespace's Role.
	grouped := make(map[string][]audiciav1alpha1.ObservedRule)
	for _, r := range filteredRules {
		grouped[r.Namespace] = append(grouped[r.Namespace], r)
	}

	// Single namespace (or only cluster-scoped): simple path.
	if len(grouped) == 1 {
		for ns, nsRules := range grouped {
			kind := "Role"
			if ns == "" {
				kind = "ClusterRole"
			}
			return e.generateSingleScope(kind, ns, subject, nsRules), nil
		}
	}

	// Multiple namespaces: generate per-namespace Role+RoleBinding pairs.
	var manifests []string
	clusterRules := grouped[""]
	delete(grouped, "")

	for ns, nsRules := range grouped {
		// Merge cluster-scoped rules into each namespace Role.
		// Copy nsRules to avoid mutating the original slice's backing array.
		allRules := make([]audiciav1alpha1.ObservedRule, 0, len(nsRules)+len(clusterRules))
		allRules = append(allRules, nsRules...)
		allRules = append(allRules, clusterRules...)
		nameBase := fmt.Sprintf("suggested-%s-%s", sanitizeForName(subject.Name), ns)
		roleName := nameBase + "-role"

		manifests = append(manifests, e.renderRole("Role", roleName, ns, allRules))
		manifests = append(manifests, e.renderBinding("Role", roleName, ns, subject))
	}

	// Only cluster-scoped rules with no namespaced rules.
	if len(grouped) == 0 && len(clusterRules) > 0 {
		return e.generateSingleScope("ClusterRole", "", subject, clusterRules), nil
	}

	return manifests, nil
}

// generateSingleScope renders a single Role/ClusterRole + Binding pair.
func (e *Engine) generateSingleScope(kind, namespace string, subject audiciav1alpha1.Subject, rules []audiciav1alpha1.ObservedRule) []string {
	roleName := fmt.Sprintf("suggested-%s-role", sanitizeForName(subject.Name))
	return []string{
		e.renderRole(kind, roleName, namespace, rules),
		e.renderBinding(kind, roleName, namespace, subject),
	}
}

// generatePerNamespace groups rules by their observed namespace and generates
// one Role+RoleBinding per target namespace. Cluster-scoped rules (empty
// namespace) and non-resource URLs get a ClusterRole.
func (e *Engine) generatePerNamespace(subject audiciav1alpha1.Subject, rules []audiciav1alpha1.ObservedRule) []string {
	grouped := groupByNamespace(rules, subject.Namespace)

	// Single namespace: simple path.
	if len(grouped) == 1 {
		for ns, nsRules := range grouped {
			kind := roleKindForNamespace(ns)
			return e.generateSingleScope(kind, ns, subject, nsRules)
		}
	}

	var manifests []string

	// Non-resource URLs (namespace key "") get a ClusterRole.
	if clusterRules, ok := grouped[""]; ok {
		nameBase := fmt.Sprintf("suggested-%s-cluster", sanitizeForName(subject.Name))
		roleName := nameBase + "-role"
		manifests = append(manifests, e.renderRole("ClusterRole", roleName, "", clusterRules))
		manifests = append(manifests, e.renderBinding("ClusterRole", roleName, "", subject))
		delete(grouped, "")
	}

	// Sort namespace keys for deterministic output.
	nsKeys := make([]string, 0, len(grouped))
	for ns := range grouped {
		nsKeys = append(nsKeys, ns)
	}
	sort.Strings(nsKeys)

	for _, ns := range nsKeys {
		nsRules := grouped[ns]
		nameBase := fmt.Sprintf("suggested-%s", sanitizeForName(subject.Name))
		if ns != subject.Namespace {
			nameBase = fmt.Sprintf("suggested-%s-%s", sanitizeForName(subject.Name), sanitizeForName(ns))
		}
		roleName := nameBase + "-role"
		manifests = append(manifests, e.renderRole("Role", roleName, ns, nsRules))
		manifests = append(manifests, e.renderBinding("Role", roleName, ns, subject))
	}

	return manifests
}

// groupByNamespace partitions rules by namespace, defaulting cluster-scoped
// resource rules to the subject's home namespace.
func groupByNamespace(rules []audiciav1alpha1.ObservedRule, homeNS string) map[string][]audiciav1alpha1.ObservedRule {
	grouped := make(map[string][]audiciav1alpha1.ObservedRule)
	for _, r := range rules {
		ns := r.Namespace
		if ns == "" && len(r.NonResourceURLs) == 0 {
			ns = homeNS
		}
		grouped[ns] = append(grouped[ns], r)
	}
	return grouped
}

// roleKindForNamespace returns "ClusterRole" for cluster-scoped (empty ns) or "Role" otherwise.
func roleKindForNamespace(ns string) string {
	if ns == "" {
		return "ClusterRole"
	}
	return "Role"
}

// sanitizeForName produces a Kubernetes-name-safe string.
func sanitizeForName(name string) string {
	s := strings.ToLower(name)
	s = strings.ReplaceAll(s, "@", "-at-")
	s = strings.ReplaceAll(s, ":", "-")
	s = strings.ReplaceAll(s, "/", "-")
	s = strings.ReplaceAll(s, ".", "-")
	if len(s) > 50 {
		s = s[:50]
	}
	return strings.TrimRight(s, "-")
}

func (e *Engine) filterVerbs(rules []audiciav1alpha1.ObservedRule) []audiciav1alpha1.ObservedRule {
	result := make([]audiciav1alpha1.ObservedRule, 0, len(rules))
	for _, r := range rules {
		filtered := r
		var validVerbs []string
		for _, v := range r.Verbs {
			if allowedVerbs[v] {
				validVerbs = append(validVerbs, v)
			}
		}
		if len(validVerbs) > 0 {
			filtered.Verbs = validVerbs
			result = append(result, filtered)
		}
	}
	return result
}

// mergeKey groups ObservedRules by identity (everything except verb).
type mergeKey struct {
	APIGroup       string
	Resource       string
	NonResourceURL string
	Namespace      string
}

// mergedRule tracks a rule being merged with its accumulated verb set.
type mergedRule struct {
	rule  audiciav1alpha1.ObservedRule
	verbs map[string]bool
}

// mergeVerbs collapses rules that share the same (apiGroup, resource, namespace)
// into a single rule with merged verb lists. Only active in Smart mode.
func (e *Engine) mergeVerbs(rules []audiciav1alpha1.ObservedRule) []audiciav1alpha1.ObservedRule {
	if e.VerbMerge != audiciav1alpha1.VerbMergeSmart {
		return rules
	}

	groups := make(map[mergeKey]*mergedRule)
	var order []mergeKey

	for _, r := range rules {
		key := mergeKeyForRule(r)
		if existing, ok := groups[key]; ok {
			mergeInto(existing, r)
		} else {
			groups[key] = newMergedRule(r)
			order = append(order, key)
		}
	}

	return flattenMerged(groups, order)
}

// mergeKeyForRule builds the deduplication key for an ObservedRule.
func mergeKeyForRule(r audiciav1alpha1.ObservedRule) mergeKey {
	key := mergeKey{Namespace: r.Namespace}
	if len(r.NonResourceURLs) > 0 {
		key.NonResourceURL = r.NonResourceURLs[0]
	} else {
		if len(r.APIGroups) > 0 {
			key.APIGroup = r.APIGroups[0]
		}
		if len(r.Resources) > 0 {
			key.Resource = r.Resources[0]
		}
	}
	return key
}

// newMergedRule creates a new mergedRule from an ObservedRule.
func newMergedRule(r audiciav1alpha1.ObservedRule) *mergedRule {
	verbSet := make(map[string]bool, len(r.Verbs))
	for _, v := range r.Verbs {
		verbSet[v] = true
	}
	return &mergedRule{rule: r, verbs: verbSet}
}

// mergeInto folds a rule's verbs and timestamps into an existing merged entry.
func mergeInto(existing *mergedRule, r audiciav1alpha1.ObservedRule) {
	for _, v := range r.Verbs {
		existing.verbs[v] = true
	}
	if r.FirstSeen.Before(&existing.rule.FirstSeen) {
		existing.rule.FirstSeen = r.FirstSeen
	}
	if existing.rule.LastSeen.Before(&r.LastSeen) {
		existing.rule.LastSeen = r.LastSeen
	}
	existing.rule.Count += r.Count
}

// flattenMerged converts merged groups back into a sorted ObservedRule slice.
func flattenMerged(groups map[mergeKey]*mergedRule, order []mergeKey) []audiciav1alpha1.ObservedRule {
	result := make([]audiciav1alpha1.ObservedRule, 0, len(order))
	for _, key := range order {
		m := groups[key]
		verbSlice := make([]string, 0, len(m.verbs))
		for v := range m.verbs {
			verbSlice = append(verbSlice, v)
		}
		sort.Strings(verbSlice)
		m.rule.Verbs = verbSlice
		result = append(result, m.rule)
	}
	return result
}

// standardVerbCount is the number of standard Kubernetes API verbs.
const standardVerbCount = 8

// applyWildcards replaces complete verb sets with ["*"] in Safe mode.
// In Forbidden mode (default), this is a no-op.
func (e *Engine) applyWildcards(rules []audiciav1alpha1.ObservedRule) []audiciav1alpha1.ObservedRule {
	if e.Wildcards != audiciav1alpha1.WildcardModeSafe {
		return rules
	}

	result := make([]audiciav1alpha1.ObservedRule, len(rules))
	for i, r := range rules {
		result[i] = r
		if len(r.NonResourceURLs) > 0 {
			continue
		}
		if hasAllStandardVerbs(r.Verbs) {
			result[i].Verbs = []string{"*"}
		}
	}
	return result
}

// hasAllStandardVerbs checks whether a verb list contains all standard Kubernetes API verbs.
func hasAllStandardVerbs(verbs []string) bool {
	if len(verbs) < standardVerbCount {
		return false
	}
	present := make(map[string]bool, len(verbs))
	for _, v := range verbs {
		present[v] = true
	}
	for v := range allowedVerbs {
		if !present[v] {
			return false
		}
	}
	return true
}

func (e *Engine) renderRole(kind, name, namespace string, rules []audiciav1alpha1.ObservedRule) string {
	// Convert ObservedRules into RBAC PolicyRules, deduplicating rules that
	// are identical after dropping the namespace (which PolicyRule doesn't have).
	seen := make(map[string]bool)
	var policyRules []rbacv1.PolicyRule
	for _, r := range rules {
		var pr rbacv1.PolicyRule
		if len(r.NonResourceURLs) > 0 {
			pr = rbacv1.PolicyRule{
				NonResourceURLs: r.NonResourceURLs,
				Verbs:           r.Verbs,
			}
		} else {
			pr = rbacv1.PolicyRule{
				APIGroups: r.APIGroups,
				Resources: r.Resources,
				Verbs:     r.Verbs,
			}
		}
		key := policyRuleKey(pr)
		if seen[key] {
			continue
		}
		seen[key] = true
		policyRules = append(policyRules, pr)
	}

	if kind == "ClusterRole" {
		obj := rbacv1.ClusterRole{
			TypeMeta: metav1.TypeMeta{
				APIVersion: rbacAPIVersion,
				Kind:       "ClusterRole",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
			Rules: policyRules,
		}
		data, err := yaml.Marshal(obj)
		if err != nil {
			return ""
		}
		return string(data)
	}

	obj := rbacv1.Role{
		TypeMeta: metav1.TypeMeta{
			APIVersion: rbacAPIVersion,
			Kind:       "Role",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Rules: policyRules,
	}
	data, err := yaml.Marshal(obj)
	if err != nil {
		return ""
	}
	return string(data)
}

func (e *Engine) renderBinding(kind, roleName, namespace string, subject audiciav1alpha1.Subject) string {
	bindingName := strings.Replace(roleName, "-role", "-binding", 1)

	// Build the RBAC subject.
	rbacSubject := rbacv1.Subject{
		Kind: string(subject.Kind),
		Name: subject.Name,
	}
	switch subject.Kind {
	case audiciav1alpha1.SubjectKindServiceAccount:
		rbacSubject.Namespace = subject.Namespace
	case audiciav1alpha1.SubjectKindUser, audiciav1alpha1.SubjectKindGroup:
		rbacSubject.APIGroup = rbacAPIGroup
	}

	if kind == "ClusterRole" {
		obj := rbacv1.ClusterRoleBinding{
			TypeMeta: metav1.TypeMeta{
				APIVersion: rbacAPIVersion,
				Kind:       "ClusterRoleBinding",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: bindingName,
			},
			RoleRef: rbacv1.RoleRef{
				APIGroup: rbacAPIGroup,
				Kind:     "ClusterRole",
				Name:     roleName,
			},
			Subjects: []rbacv1.Subject{rbacSubject},
		}
		data, err := yaml.Marshal(obj)
		if err != nil {
			return ""
		}
		return string(data)
	}

	obj := rbacv1.RoleBinding{
		TypeMeta: metav1.TypeMeta{
			APIVersion: rbacAPIVersion,
			Kind:       "RoleBinding",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      bindingName,
			Namespace: namespace,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacAPIGroup,
			Kind:     "Role",
			Name:     roleName,
		},
		Subjects: []rbacv1.Subject{rbacSubject},
	}
	data, err := yaml.Marshal(obj)
	if err != nil {
		return ""
	}
	return string(data)
}

// policyRuleKey returns a stable string key for deduplicating PolicyRules.
func policyRuleKey(pr rbacv1.PolicyRule) string {
	return strings.Join(pr.APIGroups, ",") + "|" +
		strings.Join(pr.Resources, ",") + "|" +
		strings.Join(pr.Verbs, ",") + "|" +
		strings.Join(pr.NonResourceURLs, ",")
}
