package gcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	auditv1 "k8s.io/apiserver/pkg/apis/audit/v1"
)

// logEntry represents a Cloud Logging LogEntry as received from a
// Pub/Sub sink. GKE audit events are routed through Cloud Logging
// and arrive in this format rather than as raw K8s audit events.
type logEntry struct {
	ProtoPayload *protoPayload `json:"protoPayload"`
	InsertID     string        `json:"insertId"`
	Resource     *logResource  `json:"resource"`
	Timestamp    string        `json:"timestamp"`
	Severity     string        `json:"severity"`
	LogName      string        `json:"logName"`
}

type protoPayload struct {
	Type               string              `json:"@type"`
	Status             *rpcStatus          `json:"status"`
	AuthenticationInfo *authenticationInfo `json:"authenticationInfo"`
	RequestMetadata    *requestMetadata    `json:"requestMetadata"`
	ServiceName        string              `json:"serviceName"`
	MethodName         string              `json:"methodName"`
	AuthorizationInfo  []authorizationInfo `json:"authorizationInfo"`
	ResourceName       string              `json:"resourceName"`
}

type rpcStatus struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type authenticationInfo struct {
	PrincipalEmail string `json:"principalEmail"`
}

type requestMetadata struct {
	CallerIP string `json:"callerIp"`
}

type authorizationInfo struct {
	Resource   string `json:"resource"`
	Permission string `json:"permission"`
	Granted    bool   `json:"granted"`
}

type logResource struct {
	Type   string            `json:"type"`
	Labels map[string]string `json:"labels"`
}

// groupPrefixMap maps the GKE method name group prefix to the
// canonical Kubernetes API group name. "core" maps to the empty
// string (core API group). Unknown prefixes fall back to prefix.k8s.io.
var groupPrefixMap = map[string]string{
	"core":                  "",
	"apps":                  "apps",
	"batch":                 "batch",
	"policy":                "policy",
	"autoscaling":           "autoscaling",
	"authorization":         "authorization.k8s.io",
	"authentication":        "authentication.k8s.io",
	"rbac.authorization":    "rbac.authorization.k8s.io",
	"admissionregistration": "admissionregistration.k8s.io",
	"apiextensions":         "apiextensions.k8s.io",
	"networking":            "networking.k8s.io",
	"storage":               "storage.k8s.io",
	"certificates":          "certificates.k8s.io",
	"coordination":          "coordination.k8s.io",
	"events":                "events.k8s.io",
	"discovery":             "discovery.k8s.io",
	"scheduling":            "scheduling.k8s.io",
	"node":                  "node.k8s.io",
	"flowcontrol":           "flowcontrol.apiserver.k8s.io",
}

// parseLogEntry parses a Cloud Logging LogEntry (from a GKE audit log
// routed via Pub/Sub) and converts it to a Kubernetes audit event.
//
// As a fallback, raw Kubernetes audit events (e.g., from Fluentd/Vector
// pipelines) are auto-detected and passed through unchanged.
func parseLogEntry(body []byte) ([]auditv1.Event, error) {
	if len(body) == 0 {
		return nil, nil
	}

	// Auto-detect raw K8s audit events: look for auditID without protoPayload.
	if isRawK8sAuditEvent(body) {
		return parseRawK8sEvent(body)
	}

	var entry logEntry
	if err := json.Unmarshal(body, &entry); err != nil {
		return nil, fmt.Errorf("unmarshaling Cloud Logging entry: %w", err)
	}

	// Skip entries that are not Kubernetes audit events.
	if entry.ProtoPayload == nil {
		return nil, nil
	}
	if entry.ProtoPayload.ServiceName != "k8s.io" {
		return nil, nil
	}

	event := convertLogEntry(entry)

	return []auditv1.Event{event}, nil
}

// convertLogEntry converts a single Cloud Logging LogEntry to a
// Kubernetes audit event.
func convertLogEntry(entry logEntry) auditv1.Event {
	event := auditv1.Event{}
	event.APIVersion = "audit.k8s.io/v1"
	event.Kind = "Event"
	event.Level = auditv1.LevelMetadata
	event.Stage = auditv1.StageResponseComplete

	// AuditID from insertId.
	event.AuditID = types.UID(entry.InsertID)

	// Timestamp.
	setTimestamp(&event, entry.Timestamp)

	pp := entry.ProtoPayload

	// User identity.
	if pp.AuthenticationInfo != nil && pp.AuthenticationInfo.PrincipalEmail != "" {
		event.User.Username = pp.AuthenticationInfo.PrincipalEmail
	}

	// Source IPs.
	if pp.RequestMetadata != nil && pp.RequestMetadata.CallerIP != "" {
		event.SourceIPs = []string{pp.RequestMetadata.CallerIP}
	}

	// Parse methodName to extract verb, resource, group, version.
	if pp.MethodName != "" {
		verb, resource, apiGroup, apiVersion, err := parseMethodName(pp.MethodName)
		if err == nil {
			event.Verb = verb
			event.ObjectRef = &auditv1.ObjectReference{
				Resource:   resource,
				APIGroup:   apiGroup,
				APIVersion: apiVersion,
			}
		}
	}

	// Parse resourceName to extract namespace and name.
	if pp.ResourceName != "" && event.ObjectRef != nil {
		ns, _, name := parseResourceName(pp.ResourceName)
		event.ObjectRef.Namespace = ns
		event.ObjectRef.Name = name
	}

	// Reconstruct RequestURI.
	if event.ObjectRef != nil {
		event.RequestURI = buildRequestURI(
			event.ObjectRef.APIGroup,
			event.ObjectRef.APIVersion,
			event.ObjectRef.Namespace,
			event.ObjectRef.Resource,
			event.ObjectRef.Name,
		)
	}

	// Response status.
	setResponseStatus(&event, pp.Status)

	// Annotations for traceability.
	setAnnotations(&event, entry.LogName, entry.InsertID)

	return event
}

// setTimestamp parses a RFC3339Nano timestamp string and sets it on the event.
func setTimestamp(event *auditv1.Event, ts string) {
	if ts == "" {
		return
	}
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return
	}
	mt := metav1.NewMicroTime(t)
	event.RequestReceivedTimestamp = mt
	event.StageTimestamp = mt
}

// setResponseStatus maps a gRPC status to a Kubernetes response status.
func setResponseStatus(event *auditv1.Event, status *rpcStatus) {
	if status == nil {
		return
	}
	event.ResponseStatus = &metav1.Status{Code: mapStatusCode(status.Code)}
	if status.Message != "" {
		event.ResponseStatus.Message = status.Message
	}
}

// setAnnotations adds GCP traceability annotations to the event.
func setAnnotations(event *auditv1.Event, logName, insertID string) {
	event.Annotations = map[string]string{}
	if logName != "" {
		event.Annotations["gcp.audicia.io/log-name"] = logName
	}
	if insertID != "" {
		event.Annotations["gcp.audicia.io/insert-id"] = insertID
	}
}

// parseMethodName extracts verb, resource, API group, and API version from
// a GKE method name. GKE method names follow the pattern:
//
//	io.k8s.{groupPrefix}.{version}.{resource}.{verb}
//
// Examples:
//
//	io.k8s.core.v1.pods.list                              → list, pods, "", v1
//	io.k8s.apps.v1.deployments.create                     → create, deployments, apps, v1
//	io.k8s.rbac.authorization.v1.clusterroles.list        → list, clusterroles, rbac.authorization.k8s.io, v1
//	io.k8s.authorization.v1.subjectaccessreviews.create   → create, subjectaccessreviews, authorization.k8s.io, v1
func parseMethodName(method string) (verb, resource, apiGroup, apiVersion string, err error) {
	parts := strings.Split(method, ".")
	if len(parts) < 5 || parts[0] != "io" || parts[1] != "k8s" {
		return "", "", "", "", fmt.Errorf("unexpected method format: %s", method)
	}

	// Remove "io.k8s." prefix.
	parts = parts[2:]
	// Verb is always the last segment.
	verb = parts[len(parts)-1]
	// Resource is second to last.
	resource = parts[len(parts)-2]
	// Remaining segments contain group prefix and version.
	remaining := parts[:len(parts)-2]

	// Find the version segment (matches v\d+...).
	versionIdx := -1
	for i, p := range remaining {
		if isVersionSegment(p) {
			versionIdx = i
			break
		}
	}
	if versionIdx < 0 {
		return "", "", "", "", fmt.Errorf("no version found in method: %s", method)
	}

	apiVersion = remaining[versionIdx]
	groupPrefix := strings.Join(remaining[:versionIdx], ".")
	apiGroup = mapGroupPrefix(groupPrefix)

	return verb, resource, apiGroup, apiVersion, nil
}

// isVersionSegment returns true if s looks like a Kubernetes API version
// (e.g., "v1", "v1beta1", "v2alpha1").
func isVersionSegment(s string) bool {
	return len(s) >= 2 && s[0] == 'v' && s[1] >= '0' && s[1] <= '9'
}

// mapGroupPrefix maps a GKE method name group prefix to the canonical
// Kubernetes API group name. Unknown prefixes fall back to prefix.k8s.io.
func mapGroupPrefix(prefix string) string {
	if mapped, ok := groupPrefixMap[prefix]; ok {
		return mapped
	}
	if prefix != "" {
		return prefix + ".k8s.io"
	}
	return ""
}

// parseResourceName extracts namespace, resource type, and resource name
// from a GKE resource name. Resource names follow patterns like:
//
//	core/v1/namespaces/default/pods/nginx-abc123       → ns=default, res=pods, name=nginx-abc123
//	apps/v1/namespaces/kube-system/deployments/coredns → ns=kube-system, res=deployments, name=coredns
//	core/v1/nodes/node-1                               → ns="", res=nodes, name=node-1
func parseResourceName(name string) (namespace, resourceType, resourceName string) {
	if name == "" {
		return "", "", ""
	}

	parts := strings.Split(name, "/")

	// Look for "namespaces" marker to identify namespaced resources.
	for i, p := range parts {
		if p == "namespaces" && i+1 < len(parts) {
			namespace = parts[i+1]
			// After namespace: resource[/name]
			rest := parts[i+2:]
			if len(rest) >= 1 {
				resourceType = rest[0]
			}
			if len(rest) >= 2 {
				resourceName = rest[1]
			}
			return namespace, resourceType, resourceName
		}
	}

	// Cluster-scoped: {group}/{version}/{resource}[/{name}]
	if len(parts) >= 3 {
		resourceType = parts[2]
		if len(parts) >= 4 {
			resourceName = parts[3]
		}
	}

	return "", resourceType, resourceName
}

// buildRequestURI reconstructs a Kubernetes-style request URI from the
// parsed components.
func buildRequestURI(apiGroup, apiVersion, namespace, resource, name string) string {
	if resource == "" {
		return ""
	}

	var b strings.Builder

	// Core group uses /api/, named groups use /apis/.
	if apiGroup == "" {
		b.WriteString("/api/")
		b.WriteString(apiVersion)
	} else {
		b.WriteString("/apis/")
		b.WriteString(apiGroup)
		b.WriteString("/")
		b.WriteString(apiVersion)
	}

	if namespace != "" {
		b.WriteString("/namespaces/")
		b.WriteString(namespace)
	}

	b.WriteString("/")
	b.WriteString(resource)

	if name != "" {
		b.WriteString("/")
		b.WriteString(name)
	}

	return b.String()
}

// mapStatusCode maps a gRPC/Cloud Audit status code to an HTTP status code.
func mapStatusCode(code int) int32 {
	switch code {
	case 0: // OK
		return 200
	case 1: // CANCELLED
		return 499
	case 2: // UNKNOWN
		return 500
	case 3: // INVALID_ARGUMENT
		return 400
	case 4: // DEADLINE_EXCEEDED
		return 504
	case 5: // NOT_FOUND
		return 404
	case 6: // ALREADY_EXISTS
		return 409
	case 7: // PERMISSION_DENIED
		return 403
	case 8: // RESOURCE_EXHAUSTED
		return 429
	case 9: // FAILED_PRECONDITION
		return 400
	case 10: // ABORTED
		return 409
	case 11: // OUT_OF_RANGE
		return 400
	case 12: // UNIMPLEMENTED
		return 501
	case 13: // INTERNAL
		return 500
	case 14: // UNAVAILABLE
		return 503
	case 15: // DATA_LOSS
		return 500
	case 16: // UNAUTHENTICATED
		return 401
	default:
		return 500
	}
}

// isRawK8sAuditEvent uses a fast heuristic to detect whether the body is
// a raw Kubernetes audit event (or array of events) rather than a Cloud
// Logging LogEntry. This supports custom pipelines (Fluentd/Vector →
// Pub/Sub) that forward raw K8s audit events without going through Cloud
// Logging.
func isRawK8sAuditEvent(body []byte) bool {
	if len(body) == 0 {
		return false
	}

	// JSON arrays are assumed to be raw K8s events if they don't look
	// like Cloud Logging entries (Cloud Logging sinks send individual
	// LogEntry objects, not arrays).
	if body[0] == '[' {
		return true
	}

	// Quick heuristic: check first 512 bytes for "auditID" without "protoPayload".
	window := body
	if len(window) > 512 {
		window = window[:512]
	}
	hasAuditID := bytes.Contains(window, []byte(`"auditID"`))
	hasProtoPayload := bytes.Contains(window, []byte(`"protoPayload"`))

	return hasAuditID && !hasProtoPayload
}

// parseRawK8sEvent parses raw Kubernetes audit events (single or array).
// This is the fallback path for custom pipelines.
func parseRawK8sEvent(body []byte) ([]auditv1.Event, error) {
	if len(body) > 0 && body[0] == '[' {
		var events []auditv1.Event
		if err := json.Unmarshal(body, &events); err != nil {
			return nil, fmt.Errorf("unmarshaling raw audit event array: %w", err)
		}
		return events, nil
	}

	var event auditv1.Event
	if err := json.Unmarshal(body, &event); err != nil {
		return nil, fmt.Errorf("unmarshaling raw audit event: %w", err)
	}
	return []auditv1.Event{event}, nil
}
