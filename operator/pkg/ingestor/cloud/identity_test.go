package cloud

import (
	"testing"

	auditv1 "k8s.io/apiserver/pkg/apis/audit/v1"
)

func TestClusterIdentityValidator(t *testing.T) {
	tests := []struct {
		name             string
		expectedIdentity string
		event            auditv1.Event
		want             bool
	}{
		{
			name:             "empty identity always matches",
			expectedIdentity: "",
			event:            auditv1.Event{AuditID: "a1"},
			want:             true,
		},
		{
			name:             "identity found in annotations",
			expectedIdentity: "/subscriptions/abc/resourceGroups/rg/providers/Microsoft.ContainerService/managedClusters/mycluster",
			event: auditv1.Event{
				AuditID: "a1",
				Annotations: map[string]string{
					"cluster.azure.com/resource-id": "/subscriptions/abc/resourceGroups/rg/providers/Microsoft.ContainerService/managedClusters/mycluster",
				},
			},
			want: true,
		},
		{
			name:             "identity found in request URI",
			expectedIdentity: "mycluster",
			event: auditv1.Event{
				AuditID:    "a1",
				RequestURI: "/api/v1/namespaces/default/pods?cluster=mycluster",
			},
			want: true,
		},
		{
			name:             "identity not found defaults to allow",
			expectedIdentity: "/subscriptions/abc/managedClusters/mycluster",
			event: auditv1.Event{
				AuditID:    "a1",
				RequestURI: "/api/v1/pods",
			},
			want: true, // defense-in-depth: default allow in v1
		},
		{
			name:             "identity as substring in annotation value",
			expectedIdentity: "mycluster",
			event: auditv1.Event{
				AuditID: "a1",
				Annotations: map[string]string{
					"source": "aks-mycluster-audit",
				},
			},
			want: true,
		},
		{
			name:             "nil annotations handled",
			expectedIdentity: "mycluster",
			event: auditv1.Event{
				AuditID: "a1",
			},
			want: true, // default allow
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &ClusterIdentityValidator{ExpectedIdentity: tt.expectedIdentity}
			got := v.Matches(tt.event)
			if got != tt.want {
				t.Errorf("Matches() = %v, want %v", got, tt.want)
			}
		})
	}
}
