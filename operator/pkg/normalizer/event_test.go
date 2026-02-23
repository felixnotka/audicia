package normalizer

import "testing"

func TestNormalizeEvent_BasicResource(t *testing.T) {
	rule := NormalizeEvent("pods", "", "", "get", "default", "/api/v1/namespaces/default/pods", true)
	if rule.Resource != "pods" {
		t.Errorf("Resource = %q, want pods", rule.Resource)
	}
	if rule.Verb != "get" {
		t.Errorf("Verb = %q, want get", rule.Verb)
	}
	if rule.Namespace != "default" {
		t.Errorf("Namespace = %q, want default", rule.Namespace)
	}
	if rule.APIGroup != "" {
		t.Errorf("APIGroup = %q, want empty (core group)", rule.APIGroup)
	}
	if rule.NonResourceURL != "" {
		t.Errorf("NonResourceURL = %q, want empty", rule.NonResourceURL)
	}
}

func TestNormalizeEvent_SubresourceConcatenation(t *testing.T) {
	rule := NormalizeEvent("pods", "exec", "", "create", "prod", "", true)
	if rule.Resource != "pods/exec" {
		t.Errorf("Resource = %q, want pods/exec", rule.Resource)
	}
}

func TestNormalizeEvent_SubresourceLog(t *testing.T) {
	rule := NormalizeEvent("pods", "log", "", "get", "default", "", true)
	if rule.Resource != "pods/log" {
		t.Errorf("Resource = %q, want pods/log", rule.Resource)
	}
}

func TestNormalizeEvent_SubresourceStatus(t *testing.T) {
	rule := NormalizeEvent("deployments", "status", "apps", "update", "prod", "", true)
	if rule.Resource != "deployments/status" {
		t.Errorf("Resource = %q, want deployments/status", rule.Resource)
	}
	if rule.APIGroup != "apps" {
		t.Errorf("APIGroup = %q, want apps", rule.APIGroup)
	}
}

func TestNormalizeEvent_APIGroupMigration_Extensions(t *testing.T) {
	rule := NormalizeEvent("deployments", "", "extensions", "list", "default", "", true)
	if rule.APIGroup != "apps" {
		t.Errorf("APIGroup = %q, want apps (migrated from extensions)", rule.APIGroup)
	}
}

func TestNormalizeEvent_APIGroupNoMigration(t *testing.T) {
	rule := NormalizeEvent("roles", "", "rbac.authorization.k8s.io", "get", "default", "", true)
	if rule.APIGroup != "rbac.authorization.k8s.io" {
		t.Errorf("APIGroup = %q, want rbac.authorization.k8s.io (no migration)", rule.APIGroup)
	}
}

func TestNormalizeEvent_NonResourceURL(t *testing.T) {
	rule := NormalizeEvent("", "", "", "get", "", "/metrics", false)
	if rule.NonResourceURL != "/metrics" {
		t.Errorf("NonResourceURL = %q, want /metrics", rule.NonResourceURL)
	}
	if rule.Verb != "get" {
		t.Errorf("Verb = %q, want get", rule.Verb)
	}
	if rule.Resource != "" {
		t.Errorf("Resource = %q, want empty for non-resource URL", rule.Resource)
	}
	if rule.Namespace != "" {
		t.Errorf("Namespace = %q, want empty for non-resource URL", rule.Namespace)
	}
	if rule.APIGroup != "" {
		t.Errorf("APIGroup = %q, want empty for non-resource URL", rule.APIGroup)
	}
}

func TestNormalizeEvent_NonResourceURL_Healthz(t *testing.T) {
	rule := NormalizeEvent("", "", "", "get", "", "/healthz", false)
	if rule.NonResourceURL != "/healthz" {
		t.Errorf("NonResourceURL = %q, want /healthz", rule.NonResourceURL)
	}
}

func TestNormalizeEvent_NonResourceURL_APIDiscovery(t *testing.T) {
	rule := NormalizeEvent("", "", "", "get", "", "/api/v1", false)
	if rule.NonResourceURL != "/api/v1" {
		t.Errorf("NonResourceURL = %q, want /api/v1", rule.NonResourceURL)
	}
}

func TestNormalizeEvent_ClusterScopedResource(t *testing.T) {
	rule := NormalizeEvent("namespaces", "", "", "list", "", "", true)
	if rule.Resource != "namespaces" {
		t.Errorf("Resource = %q, want namespaces", rule.Resource)
	}
	if rule.Namespace != "" {
		t.Errorf("Namespace = %q, want empty for cluster-scoped", rule.Namespace)
	}
}

func TestNormalizeEvent_EmptySubresource(t *testing.T) {
	rule := NormalizeEvent("configmaps", "", "", "get", "default", "", true)
	if rule.Resource != "configmaps" {
		t.Errorf("Resource = %q, want configmaps (no subresource concatenation)", rule.Resource)
	}
}

func TestNormalizeEvent_HasObjectRefTrue_IgnoresRequestURI(t *testing.T) {
	// When hasObjectRef is true, the function uses the resource fields, not requestURI.
	rule := NormalizeEvent("pods", "", "", "get", "default", "/api/v1/namespaces/default/pods", true)
	if rule.NonResourceURL != "" {
		t.Errorf("should not set NonResourceURL when hasObjectRef=true")
	}
	if rule.Resource != "pods" {
		t.Errorf("Resource = %q, want pods", rule.Resource)
	}
}

func TestNormalizeEvent_HasObjectRefFalse_EmptyURI(t *testing.T) {
	// No objectRef and no requestURI — falls through to resource path with empty fields.
	rule := NormalizeEvent("", "", "", "get", "", "", false)
	if rule.NonResourceURL != "" {
		t.Errorf("NonResourceURL = %q, want empty (no requestURI)", rule.NonResourceURL)
	}
	if rule.Resource != "" {
		t.Errorf("Resource = %q, want empty", rule.Resource)
	}
}

func TestNormalizeEvent_APIGroupMigration_DoesNotAffectNonResourceURL(t *testing.T) {
	// Non-resource URL path should not run API group migration.
	rule := NormalizeEvent("", "", "extensions", "get", "", "/metrics", false)
	if rule.NonResourceURL != "/metrics" {
		t.Errorf("NonResourceURL = %q, want /metrics", rule.NonResourceURL)
	}
	// APIGroup should not be set for non-resource URLs.
	if rule.APIGroup != "" {
		t.Errorf("APIGroup = %q, want empty for non-resource URL", rule.APIGroup)
	}
}

func TestNormalizeEvent_MultipleSubresourceLevels(t *testing.T) {
	// Only one subresource level is concatenated.
	rule := NormalizeEvent("pods", "exec", "", "create", "default", "", true)
	if rule.Resource != "pods/exec" {
		t.Errorf("Resource = %q, want pods/exec", rule.Resource)
	}
}
