package ingestor

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	auditv1 "k8s.io/apiserver/pkg/apis/audit/v1"
)

func TestHandleAuditRequest_ValidPost(t *testing.T) {
	w := &WebhookIngestor{MaxRequestBodyBytes: 1048576}
	ch := make(chan auditv1.Event, 10)
	dedup := newDeduplicationCache(100)
	limiter := newRateLimiter(100)

	handler := w.handleAuditRequest(ch, dedup, limiter)

	eventList := auditv1.EventList{
		TypeMeta: metav1.TypeMeta{Kind: "EventList", APIVersion: "audit.k8s.io/v1"},
		Items: []auditv1.Event{
			{
				TypeMeta: metav1.TypeMeta{Kind: "Event", APIVersion: "audit.k8s.io/v1"},
				Level:    "Metadata",
				AuditID:  "test-1",
				Verb:     "get",
			},
			{
				TypeMeta: metav1.TypeMeta{Kind: "Event", APIVersion: "audit.k8s.io/v1"},
				Level:    "Metadata",
				AuditID:  "test-2",
				Verb:     "list",
			},
		},
	}
	body, _ := json.Marshal(eventList)

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}

	close(ch)
	var count int
	for range ch {
		count++
	}
	if count != 2 {
		t.Errorf("got %d events, want 2", count)
	}
}

func TestHandleAuditRequest_GetMethodRejected(t *testing.T) {
	w := &WebhookIngestor{MaxRequestBodyBytes: 1048576}
	ch := make(chan auditv1.Event, 10)
	dedup := newDeduplicationCache(100)
	limiter := newRateLimiter(100)

	handler := w.handleAuditRequest(ch, dedup, limiter)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	handler(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleAuditRequest_InvalidJSON(t *testing.T) {
	w := &WebhookIngestor{MaxRequestBodyBytes: 1048576}
	ch := make(chan auditv1.Event, 10)
	dedup := newDeduplicationCache(100)
	limiter := newRateLimiter(100)

	handler := w.handleAuditRequest(ch, dedup, limiter)

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte("not json")))
	rr := httptest.NewRecorder()

	handler(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestHandleAuditRequest_Deduplication(t *testing.T) {
	w := &WebhookIngestor{MaxRequestBodyBytes: 1048576}
	ch := make(chan auditv1.Event, 10)
	dedup := newDeduplicationCache(100)
	limiter := newRateLimiter(100)

	handler := w.handleAuditRequest(ch, dedup, limiter)

	// Send same auditID twice.
	eventList := auditv1.EventList{
		Items: []auditv1.Event{
			{AuditID: "dup-1", Verb: "get"},
			{AuditID: "dup-1", Verb: "get"},
			{AuditID: "dup-2", Verb: "list"},
		},
	}
	body, _ := json.Marshal(eventList)

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}

	close(ch)
	var count int
	for range ch {
		count++
	}
	// dup-1 should only be sent once, dup-2 once = 2 total.
	if count != 2 {
		t.Errorf("got %d events, want 2 (deduplication)", count)
	}
}

func TestDeduplicationCache_Basic(t *testing.T) {
	c := newDeduplicationCache(3)

	if c.seen("a") {
		t.Error("'a' should not be seen yet")
	}
	if !c.seen("a") {
		t.Error("'a' should be seen now")
	}
}

func TestDeduplicationCache_Eviction(t *testing.T) {
	c := newDeduplicationCache(2)

	c.seen("a")
	c.seen("b")
	c.seen("c") // Should evict "a". Cache: ["b","c"].

	// Check "b" first — it should still be present.
	// (Note: seen() is check-and-add, so checking "a" first would re-add it
	// and evict "b", making the second assertion fail.)
	if !c.seen("b") {
		t.Error("'b' should still be present")
	}
	if c.seen("a") {
		t.Error("'a' should have been evicted")
	}
}

func TestRateLimiter_AllowsWithinLimit(t *testing.T) {
	rl := newRateLimiter(10)
	for i := 0; i < 10; i++ {
		if !rl.allow() {
			t.Errorf("request %d should be allowed", i)
		}
	}
}

func TestRateLimiter_DeniesOverLimit(t *testing.T) {
	rl := newRateLimiter(1)
	rl.allow() // Consume the single token.
	if rl.allow() {
		t.Error("second request should be denied")
	}
}

func TestWebhookIngestor_Checkpoint(t *testing.T) {
	w := NewWebhookIngestor(8443, "", "")
	pos := w.Checkpoint()
	if pos.FileOffset != 0 || pos.Inode != 0 || pos.LastTimestamp != "" {
		t.Errorf("webhook checkpoint should be empty, got %+v", pos)
	}
}
