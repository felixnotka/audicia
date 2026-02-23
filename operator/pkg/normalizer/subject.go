package normalizer

import (
	"strings"

	audiciav1alpha1 "github.com/felixnotka/audicia/operator/pkg/apis/audicia.io/v1alpha1"
)

const (
	serviceAccountPrefix = "system:serviceaccount:"
)

// NormalizeSubject converts a raw Kubernetes username into a structured Subject.
// Returns the subject and whether it should be included (false = system user to skip).
func NormalizeSubject(username string, ignoreSystemUsers bool) (audiciav1alpha1.Subject, bool) {
	// Service accounts: system:serviceaccount:<namespace>:<name>
	if strings.HasPrefix(username, serviceAccountPrefix) {
		parts := strings.SplitN(strings.TrimPrefix(username, serviceAccountPrefix), ":", 2)
		if len(parts) == 2 {
			return audiciav1alpha1.Subject{
				Kind:      audiciav1alpha1.SubjectKindServiceAccount,
				Namespace: parts[0],
				Name:      parts[1],
			}, true
		}
	}

	// System users (e.g., system:kube-controller-manager, system:apiserver)
	if ignoreSystemUsers && strings.HasPrefix(username, "system:") {
		return audiciav1alpha1.Subject{}, false
	}

	// Regular users
	return audiciav1alpha1.Subject{
		Kind: audiciav1alpha1.SubjectKindUser,
		Name: username,
	}, true
}
