// Package rbac resolves the effective RBAC permissions for a Kubernetes subject
// by inspecting all RoleBindings and ClusterRoleBindings in the cluster.
package rbac

import (
	"context"
	"fmt"

	rbacv1 "k8s.io/api/rbac/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	audiciav1alpha1 "github.com/felixnotka/audicia/operator/pkg/apis/audicia.io/v1alpha1"
)

// ScopedRule is a PolicyRule with the namespace it applies in.
// Namespace="" means the rule applies cluster-wide (from a ClusterRoleBinding).
type ScopedRule struct {
	rbacv1.PolicyRule
	Namespace string
}

// Resolver resolves the effective RBAC permissions for a subject by querying
// bindings and roles from the Kubernetes API (via a caching client).
type Resolver struct {
	client client.Reader
}

// NewResolver creates a Resolver. The client should be a caching reader (e.g.,
// the controller-runtime manager client) to avoid excessive API server load.
func NewResolver(c client.Reader) *Resolver {
	return &Resolver{client: c}
}

// EffectiveRules returns all RBAC PolicyRules granted to the given subject,
// each annotated with the namespace it applies in. Cluster-wide rules
// (from ClusterRoleBindings) have Namespace="".
//
// Roles/ClusterRoles that cannot be resolved (e.g., deleted) are silently skipped.
// Aggregated ClusterRoles (label-selector-based aggregation) are NOT resolved.
func (r *Resolver) EffectiveRules(ctx context.Context, subject audiciav1alpha1.Subject) ([]ScopedRule, error) {
	var result []ScopedRule

	// 1. ClusterRoleBindings → cluster-wide scope.
	clusterRules, err := r.rulesFromClusterRoleBindings(ctx, subject)
	if err != nil {
		return nil, err
	}
	result = append(result, clusterRules...)

	// 2. RoleBindings → scoped to the RoleBinding's namespace.
	nsRules, err := r.rulesFromRoleBindings(ctx, subject)
	if err != nil {
		return nil, err
	}
	result = append(result, nsRules...)

	return result, nil
}

// rulesFromClusterRoleBindings collects cluster-wide rules from matching ClusterRoleBindings.
func (r *Resolver) rulesFromClusterRoleBindings(ctx context.Context, subject audiciav1alpha1.Subject) ([]ScopedRule, error) {
	var crbList rbacv1.ClusterRoleBindingList
	if err := r.client.List(ctx, &crbList); err != nil {
		return nil, fmt.Errorf("listing ClusterRoleBindings: %w", err)
	}

	var result []ScopedRule
	for i := range crbList.Items {
		crb := &crbList.Items[i]
		if !matchesSubject(crb.Subjects, subject) {
			continue
		}
		rules, err := r.resolveClusterRole(ctx, crb.RoleRef.Name)
		if err != nil {
			continue // Role may have been deleted; skip.
		}
		for _, pr := range rules {
			result = append(result, ScopedRule{PolicyRule: pr, Namespace: ""})
		}
	}
	return result, nil
}

// rulesFromRoleBindings collects namespace-scoped rules from matching RoleBindings.
func (r *Resolver) rulesFromRoleBindings(ctx context.Context, subject audiciav1alpha1.Subject) ([]ScopedRule, error) {
	var rbList rbacv1.RoleBindingList
	if err := r.client.List(ctx, &rbList); err != nil {
		return nil, fmt.Errorf("listing RoleBindings: %w", err)
	}

	var result []ScopedRule
	for i := range rbList.Items {
		rb := &rbList.Items[i]
		if !matchesSubject(rb.Subjects, subject) {
			continue
		}
		rules := r.resolveRoleRef(ctx, rb.Namespace, rb.RoleRef)
		for _, pr := range rules {
			result = append(result, ScopedRule{PolicyRule: pr, Namespace: rb.Namespace})
		}
	}
	return result, nil
}

// resolveRoleRef resolves a RoleRef to its PolicyRules, returning nil on error.
func (r *Resolver) resolveRoleRef(ctx context.Context, namespace string, ref rbacv1.RoleRef) []rbacv1.PolicyRule {
	var rules []rbacv1.PolicyRule
	var err error
	if ref.Kind == "ClusterRole" {
		rules, err = r.resolveClusterRole(ctx, ref.Name)
	} else {
		rules, err = r.resolveRole(ctx, namespace, ref.Name)
	}
	if err != nil {
		return nil // Role may have been deleted; skip.
	}
	return rules
}

func (r *Resolver) resolveClusterRole(ctx context.Context, name string) ([]rbacv1.PolicyRule, error) {
	var cr rbacv1.ClusterRole
	if err := r.client.Get(ctx, client.ObjectKey{Name: name}, &cr); err != nil {
		return nil, err
	}
	return cr.Rules, nil
}

func (r *Resolver) resolveRole(ctx context.Context, namespace, name string) ([]rbacv1.PolicyRule, error) {
	var role rbacv1.Role
	if err := r.client.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, &role); err != nil {
		return nil, err
	}
	return role.Rules, nil
}

// matchesSubject checks if any of the binding's subjects match the given Audicia subject.
func matchesSubject(subjects []rbacv1.Subject, target audiciav1alpha1.Subject) bool {
	for _, s := range subjects {
		switch target.Kind {
		case audiciav1alpha1.SubjectKindServiceAccount:
			if s.Kind == "ServiceAccount" && s.Name == target.Name && s.Namespace == target.Namespace {
				return true
			}
		case audiciav1alpha1.SubjectKindUser:
			if s.Kind == "User" && s.Name == target.Name {
				return true
			}
		case audiciav1alpha1.SubjectKindGroup:
			if s.Kind == "Group" && s.Name == target.Name {
				return true
			}
		}
	}
	return false
}
