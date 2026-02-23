package normalizer

// CanonicalRule represents a normalized RBAC rule derived from an audit event.
type CanonicalRule struct {
	// APIGroup is the API group (e.g., "", "apps", "rbac.authorization.k8s.io").
	APIGroup string

	// Resource is the resource, including subresource if applicable (e.g., "pods", "pods/exec").
	Resource string

	// Verb is the API verb (e.g., "get", "list", "create").
	Verb string

	// NonResourceURL is the non-resource URL (e.g., "/metrics"). Mutually exclusive with Resource.
	NonResourceURL string

	// Namespace is the target namespace (empty for cluster-scoped).
	Namespace string
}

// apiGroupMigrations maps deprecated API groups to their stable replacements.
var apiGroupMigrations = map[string]string{
	"extensions": "apps",
}

// NormalizeEvent converts raw audit event fields into a CanonicalRule.
func NormalizeEvent(resource, subresource, apiGroup, verb, namespace, requestURI string, hasObjectRef bool) CanonicalRule {
	// Non-resource URLs: objectRef is nil, use requestURI.
	if !hasObjectRef && requestURI != "" {
		return CanonicalRule{
			NonResourceURL: requestURI,
			Verb:           verb,
		}
	}

	// Migrate deprecated API groups.
	if migrated, ok := apiGroupMigrations[apiGroup]; ok {
		apiGroup = migrated
	}

	// Concatenate subresources (e.g., "pods" + "exec" -> "pods/exec").
	fullResource := resource
	if subresource != "" {
		fullResource = resource + "/" + subresource
	}

	return CanonicalRule{
		APIGroup:  apiGroup,
		Resource:  fullResource,
		Verb:      verb,
		Namespace: namespace,
	}
}
