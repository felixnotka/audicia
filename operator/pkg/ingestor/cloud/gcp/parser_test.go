package gcp

import (
	"encoding/json"
	"testing"
)

// makeLogEntry creates a minimal Cloud Logging LogEntry JSON for testing.
func makeLogEntry(methodName, principalEmail, resourceName string) []byte {
	entry := map[string]interface{}{
		"insertId":  "test-insert-id",
		"timestamp": "2024-06-15T10:30:00.123456Z",
		"severity":  "INFO",
		"logName":   "projects/my-project/logs/cloudaudit.googleapis.com%2Factivity",
		"protoPayload": map[string]interface{}{
			"@type":       "type.googleapis.com/google.cloud.audit.AuditLog",
			"serviceName": "k8s.io",
			"methodName":  methodName,
			"authenticationInfo": map[string]interface{}{
				"principalEmail": principalEmail,
			},
			"requestMetadata": map[string]interface{}{
				"callerIp": "10.0.0.1",
			},
			"resourceName": resourceName,
			"status":       map[string]interface{}{"code": 0},
		},
		"resource": map[string]interface{}{
			"type": "k8s_cluster",
			"labels": map[string]interface{}{
				"cluster_name": "my-cluster",
				"project_id":   "my-project",
				"location":     "us-central1-a",
			},
		},
	}
	b, _ := json.Marshal(entry)
	return b
}

// makeLogEntryWithService creates a Cloud Logging entry with a custom serviceName.
func makeLogEntryWithService(serviceName string) []byte {
	entry := map[string]interface{}{
		"insertId":  "test-insert-id",
		"timestamp": "2024-06-15T10:30:00Z",
		"protoPayload": map[string]interface{}{
			"@type":        "type.googleapis.com/google.cloud.audit.AuditLog",
			"serviceName":  serviceName,
			"methodName":   "some.method",
			"resourceName": "some/resource",
		},
	}
	b, _ := json.Marshal(entry)
	return b
}

// makeLogEntryWithStatus creates a Cloud Logging entry with a custom status code.
func makeLogEntryWithStatus(statusCode int) []byte {
	entry := map[string]interface{}{
		"insertId":  "test-insert-id",
		"timestamp": "2024-06-15T10:30:00Z",
		"logName":   "projects/my-project/logs/cloudaudit.googleapis.com%2Factivity",
		"protoPayload": map[string]interface{}{
			"@type":       "type.googleapis.com/google.cloud.audit.AuditLog",
			"serviceName": "k8s.io",
			"methodName":  "io.k8s.core.v1.pods.get",
			"authenticationInfo": map[string]interface{}{
				"principalEmail": "test@example.com",
			},
			"resourceName": "core/v1/namespaces/default/pods/nginx",
			"status":       map[string]interface{}{"code": statusCode},
		},
	}
	b, _ := json.Marshal(entry)
	return b
}

// makeRawAuditEvent creates a raw K8s audit event JSON for testing the fallback path.
func makeRawAuditEvent(auditID, verb, requestURI string) []byte {
	event := map[string]interface{}{
		"auditID":    auditID,
		"verb":       verb,
		"requestURI": requestURI,
	}
	b, _ := json.Marshal(event)
	return b
}

func makeRawAuditEventArray(events ...[]byte) []byte {
	result := []byte("[")
	for i, e := range events {
		if i > 0 {
			result = append(result, ',')
		}
		result = append(result, e...)
	}
	result = append(result, ']')
	return result
}

func TestParseLogEntry(t *testing.T) {
	tests := []struct {
		name       string
		input      []byte
		wantEvents int
		wantErr    bool
	}{
		{
			name:       "valid GKE audit event",
			input:      makeLogEntry("io.k8s.core.v1.pods.list", "system:serviceaccount:kube-system:replicaset-controller", "core/v1/namespaces/default/pods"),
			wantEvents: 1,
		},
		{
			name:       "non-K8s service skipped",
			input:      makeLogEntryWithService("compute.googleapis.com"),
			wantEvents: 0,
		},
		{
			name:       "missing protoPayload skipped",
			input:      []byte(`{"insertId":"abc","timestamp":"2024-01-01T00:00:00Z"}`),
			wantEvents: 0,
		},
		{
			name:       "empty body",
			input:      []byte{},
			wantEvents: 0,
			wantErr:    false,
		},
		{
			name:       "nil body",
			input:      nil,
			wantEvents: 0,
			wantErr:    false,
		},
		{
			name:       "invalid JSON",
			input:      []byte("not json"),
			wantEvents: 0,
			wantErr:    true,
		},
		{
			name: "raw K8s audit event fallback",
			input: makeRawAuditEvent(
				"raw-id-1", "get", "/api/v1/pods",
			),
			wantEvents: 1,
		},
		{
			name: "raw K8s audit event array fallback",
			input: makeRawAuditEventArray(
				makeRawAuditEvent("arr-1", "get", "/api/v1/pods"),
				makeRawAuditEvent("arr-2", "list", "/api/v1/services"),
			),
			wantEvents: 2,
		},
		{
			name:       "empty JSON object without protoPayload",
			input:      []byte("{}"),
			wantEvents: 0,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events, err := parseLogEntry(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseLogEntry() error = %v, wantErr %v", err, tt.wantErr)
			}
			if len(events) != tt.wantEvents {
				t.Errorf("parseLogEntry() got %d events, want %d", len(events), tt.wantEvents)
			}
		})
	}
}

func TestParseLogEntryFieldExtraction(t *testing.T) {
	input := makeLogEntry(
		"io.k8s.apps.v1.deployments.create",
		"system:serviceaccount:default:my-sa",
		"apps/v1/namespaces/default/deployments/nginx-deploy",
	)

	events, err := parseLogEntry(input)
	if err != nil {
		t.Fatalf("parseLogEntry() error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}

	e := events[0]

	// AuditID from insertId.
	if string(e.AuditID) != "test-insert-id" {
		t.Errorf("AuditID = %q, want %q", e.AuditID, "test-insert-id")
	}

	// Verb from methodName.
	if e.Verb != "create" {
		t.Errorf("Verb = %q, want %q", e.Verb, "create")
	}

	// User from authenticationInfo.
	if e.User.Username != "system:serviceaccount:default:my-sa" {
		t.Errorf("User.Username = %q, want %q", e.User.Username, "system:serviceaccount:default:my-sa")
	}

	// SourceIPs from requestMetadata.
	if len(e.SourceIPs) != 1 || e.SourceIPs[0] != "10.0.0.1" {
		t.Errorf("SourceIPs = %v, want [10.0.0.1]", e.SourceIPs)
	}

	// ObjectRef from methodName + resourceName.
	if e.ObjectRef == nil {
		t.Fatal("ObjectRef is nil")
	}
	if e.ObjectRef.Resource != "deployments" {
		t.Errorf("ObjectRef.Resource = %q, want %q", e.ObjectRef.Resource, "deployments")
	}
	if e.ObjectRef.APIGroup != "apps" {
		t.Errorf("ObjectRef.APIGroup = %q, want %q", e.ObjectRef.APIGroup, "apps")
	}
	if e.ObjectRef.APIVersion != "v1" {
		t.Errorf("ObjectRef.APIVersion = %q, want %q", e.ObjectRef.APIVersion, "v1")
	}
	if e.ObjectRef.Namespace != "default" {
		t.Errorf("ObjectRef.Namespace = %q, want %q", e.ObjectRef.Namespace, "default")
	}
	if e.ObjectRef.Name != "nginx-deploy" {
		t.Errorf("ObjectRef.Name = %q, want %q", e.ObjectRef.Name, "nginx-deploy")
	}

	// RequestURI reconstructed.
	wantURI := "/apis/apps/v1/namespaces/default/deployments/nginx-deploy"
	if e.RequestURI != wantURI {
		t.Errorf("RequestURI = %q, want %q", e.RequestURI, wantURI)
	}

	// ResponseStatus from status.
	if e.ResponseStatus == nil {
		t.Fatal("ResponseStatus is nil")
	}
	if e.ResponseStatus.Code != 200 {
		t.Errorf("ResponseStatus.Code = %d, want %d", e.ResponseStatus.Code, 200)
	}

	// Timestamp parsed.
	if e.RequestReceivedTimestamp.IsZero() {
		t.Error("RequestReceivedTimestamp is zero")
	}

	// Annotations preserved.
	if e.Annotations["gcp.audicia.io/insert-id"] != "test-insert-id" {
		t.Errorf("annotation insert-id = %q, want %q", e.Annotations["gcp.audicia.io/insert-id"], "test-insert-id")
	}
}

func TestParseLogEntryCoreGroup(t *testing.T) {
	input := makeLogEntry(
		"io.k8s.core.v1.pods.get",
		"user@example.com",
		"core/v1/namespaces/kube-system/pods/coredns-abc",
	)

	events, err := parseLogEntry(input)
	if err != nil {
		t.Fatalf("parseLogEntry() error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}

	e := events[0]
	if e.ObjectRef == nil {
		t.Fatal("ObjectRef is nil")
	}
	if e.ObjectRef.APIGroup != "" {
		t.Errorf("ObjectRef.APIGroup = %q, want empty string (core group)", e.ObjectRef.APIGroup)
	}

	wantURI := "/api/v1/namespaces/kube-system/pods/coredns-abc"
	if e.RequestURI != wantURI {
		t.Errorf("RequestURI = %q, want %q", e.RequestURI, wantURI)
	}
}

func TestParseMethodName(t *testing.T) {
	tests := []struct {
		name      string
		method    string
		wantVerb  string
		wantRes   string
		wantGroup string
		wantVer   string
		wantErr   bool
	}{
		{
			name:      "core group pods list",
			method:    "io.k8s.core.v1.pods.list",
			wantVerb:  "list",
			wantRes:   "pods",
			wantGroup: "",
			wantVer:   "v1",
		},
		{
			name:      "core group pods get",
			method:    "io.k8s.core.v1.pods.get",
			wantVerb:  "get",
			wantRes:   "pods",
			wantGroup: "",
			wantVer:   "v1",
		},
		{
			name:      "apps group deployments create",
			method:    "io.k8s.apps.v1.deployments.create",
			wantVerb:  "create",
			wantRes:   "deployments",
			wantGroup: "apps",
			wantVer:   "v1",
		},
		{
			name:      "authorization group",
			method:    "io.k8s.authorization.v1.subjectaccessreviews.create",
			wantVerb:  "create",
			wantRes:   "subjectaccessreviews",
			wantGroup: "authorization.k8s.io",
			wantVer:   "v1",
		},
		{
			name:      "rbac.authorization multi-segment group",
			method:    "io.k8s.rbac.authorization.v1.clusterroles.list",
			wantVerb:  "list",
			wantRes:   "clusterroles",
			wantGroup: "rbac.authorization.k8s.io",
			wantVer:   "v1",
		},
		{
			name:      "networking group",
			method:    "io.k8s.networking.v1.ingresses.get",
			wantVerb:  "get",
			wantRes:   "ingresses",
			wantGroup: "networking.k8s.io",
			wantVer:   "v1",
		},
		{
			name:      "storage group",
			method:    "io.k8s.storage.v1.storageclasses.list",
			wantVerb:  "list",
			wantRes:   "storageclasses",
			wantGroup: "storage.k8s.io",
			wantVer:   "v1",
		},
		{
			name:      "batch group",
			method:    "io.k8s.batch.v1.jobs.create",
			wantVerb:  "create",
			wantRes:   "jobs",
			wantGroup: "batch",
			wantVer:   "v1",
		},
		{
			name:      "beta version",
			method:    "io.k8s.policy.v1beta1.podsecuritypolicies.list",
			wantVerb:  "list",
			wantRes:   "podsecuritypolicies",
			wantGroup: "policy",
			wantVer:   "v1beta1",
		},
		{
			name:      "apiextensions group",
			method:    "io.k8s.apiextensions.v1.customresourcedefinitions.get",
			wantVerb:  "get",
			wantRes:   "customresourcedefinitions",
			wantGroup: "apiextensions.k8s.io",
			wantVer:   "v1",
		},
		{
			name:      "unknown group falls back to .k8s.io suffix",
			method:    "io.k8s.custom.v1.widgets.delete",
			wantVerb:  "delete",
			wantRes:   "widgets",
			wantGroup: "custom.k8s.io",
			wantVer:   "v1",
		},
		{
			name:    "too short",
			method:  "io.k8s.pods",
			wantErr: true,
		},
		{
			name:    "wrong prefix",
			method:  "com.google.v1.pods.list",
			wantErr: true,
		},
		{
			name:    "no version segment",
			method:  "io.k8s.core.pods.list",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			verb, res, group, ver, err := parseMethodName(tt.method)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseMethodName(%q) error = %v, wantErr %v", tt.method, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if verb != tt.wantVerb {
				t.Errorf("verb = %q, want %q", verb, tt.wantVerb)
			}
			if res != tt.wantRes {
				t.Errorf("resource = %q, want %q", res, tt.wantRes)
			}
			if group != tt.wantGroup {
				t.Errorf("apiGroup = %q, want %q", group, tt.wantGroup)
			}
			if ver != tt.wantVer {
				t.Errorf("apiVersion = %q, want %q", ver, tt.wantVer)
			}
		})
	}
}

func TestParseResourceName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantNS   string
		wantRes  string
		wantName string
	}{
		{
			name:     "namespaced with name",
			input:    "core/v1/namespaces/default/pods/nginx-abc123",
			wantNS:   "default",
			wantRes:  "pods",
			wantName: "nginx-abc123",
		},
		{
			name:     "namespaced without name (list)",
			input:    "core/v1/namespaces/default/pods",
			wantNS:   "default",
			wantRes:  "pods",
			wantName: "",
		},
		{
			name:     "cluster-scoped with name",
			input:    "core/v1/nodes/node-1",
			wantNS:   "",
			wantRes:  "nodes",
			wantName: "node-1",
		},
		{
			name:     "cluster-scoped without name",
			input:    "core/v1/nodes",
			wantNS:   "",
			wantRes:  "nodes",
			wantName: "",
		},
		{
			name:     "apps group namespaced",
			input:    "apps/v1/namespaces/kube-system/deployments/coredns",
			wantNS:   "kube-system",
			wantRes:  "deployments",
			wantName: "coredns",
		},
		{
			name:     "empty string",
			input:    "",
			wantNS:   "",
			wantRes:  "",
			wantName: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ns, res, name := parseResourceName(tt.input)
			if ns != tt.wantNS {
				t.Errorf("namespace = %q, want %q", ns, tt.wantNS)
			}
			if res != tt.wantRes {
				t.Errorf("resourceType = %q, want %q", res, tt.wantRes)
			}
			if name != tt.wantName {
				t.Errorf("resourceName = %q, want %q", name, tt.wantName)
			}
		})
	}
}

func TestMapStatusCode(t *testing.T) {
	tests := []struct {
		grpc     int
		wantHTTP int32
	}{
		{0, 200},  // OK
		{1, 499},  // CANCELLED
		{3, 400},  // INVALID_ARGUMENT
		{5, 404},  // NOT_FOUND
		{7, 403},  // PERMISSION_DENIED
		{13, 500}, // INTERNAL
		{16, 401}, // UNAUTHENTICATED
		{14, 503}, // UNAVAILABLE
		{6, 409},  // ALREADY_EXISTS
		{8, 429},  // RESOURCE_EXHAUSTED
		{12, 501}, // UNIMPLEMENTED
		{99, 500}, // unknown maps to 500
	}

	for _, tt := range tests {
		got := mapStatusCode(tt.grpc)
		if got != tt.wantHTTP {
			t.Errorf("mapStatusCode(%d) = %d, want %d", tt.grpc, got, tt.wantHTTP)
		}
	}
}

func TestBuildRequestURI(t *testing.T) {
	tests := []struct {
		name     string
		group    string
		version  string
		ns       string
		resource string
		resName  string
		want     string
	}{
		{
			name:     "core group namespaced with name",
			group:    "",
			version:  "v1",
			ns:       "default",
			resource: "pods",
			resName:  "nginx",
			want:     "/api/v1/namespaces/default/pods/nginx",
		},
		{
			name:     "core group cluster-scoped",
			group:    "",
			version:  "v1",
			ns:       "",
			resource: "nodes",
			resName:  "node-1",
			want:     "/api/v1/nodes/node-1",
		},
		{
			name:     "named group namespaced",
			group:    "apps",
			version:  "v1",
			ns:       "kube-system",
			resource: "deployments",
			resName:  "coredns",
			want:     "/apis/apps/v1/namespaces/kube-system/deployments/coredns",
		},
		{
			name:     "named group list (no name)",
			group:    "apps",
			version:  "v1",
			ns:       "default",
			resource: "deployments",
			resName:  "",
			want:     "/apis/apps/v1/namespaces/default/deployments",
		},
		{
			name:     "empty resource",
			group:    "",
			version:  "v1",
			ns:       "",
			resource: "",
			resName:  "",
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildRequestURI(tt.group, tt.version, tt.ns, tt.resource, tt.resName)
			if got != tt.want {
				t.Errorf("buildRequestURI() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRawK8sEventFallbackFieldExtraction(t *testing.T) {
	input := makeRawAuditEvent("raw-test-id", "create", "/api/v1/namespaces/default/pods")
	events, err := parseLogEntry(input)
	if err != nil {
		t.Fatalf("parseLogEntry() error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}

	if string(events[0].AuditID) != "raw-test-id" {
		t.Errorf("AuditID = %q, want %q", events[0].AuditID, "raw-test-id")
	}
	if events[0].Verb != "create" {
		t.Errorf("Verb = %q, want %q", events[0].Verb, "create")
	}
	if events[0].RequestURI != "/api/v1/namespaces/default/pods" {
		t.Errorf("RequestURI = %q, want %q", events[0].RequestURI, "/api/v1/namespaces/default/pods")
	}
}

func TestParseLogEntryStatusCodes(t *testing.T) {
	tests := []struct {
		grpcCode     int
		wantHTTPCode int32
	}{
		{0, 200},
		{7, 403},
		{16, 401},
		{5, 404},
	}

	for _, tt := range tests {
		input := makeLogEntryWithStatus(tt.grpcCode)
		events, err := parseLogEntry(input)
		if err != nil {
			t.Fatalf("parseLogEntry() error = %v for grpc code %d", err, tt.grpcCode)
		}
		if len(events) != 1 {
			t.Fatalf("got %d events, want 1 for grpc code %d", len(events), tt.grpcCode)
		}
		if events[0].ResponseStatus == nil {
			t.Fatalf("ResponseStatus is nil for grpc code %d", tt.grpcCode)
		}
		if events[0].ResponseStatus.Code != tt.wantHTTPCode {
			t.Errorf("ResponseStatus.Code = %d, want %d for grpc code %d",
				events[0].ResponseStatus.Code, tt.wantHTTPCode, tt.grpcCode)
		}
	}
}
