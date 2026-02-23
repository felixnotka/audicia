package ingestor

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	auditv1 "k8s.io/apiserver/pkg/apis/audit/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

var webhookLog = ctrl.Log.WithName("ingestor").WithName("webhook")

// WebhookIngestor receives audit events via an HTTPS webhook endpoint.
type WebhookIngestor struct {
	// Port is the HTTPS port to listen on.
	Port int32

	// TLSCertFile is the path to the TLS certificate.
	TLSCertFile string

	// TLSKeyFile is the path to the TLS private key.
	TLSKeyFile string

	// MaxRequestBodyBytes is the maximum request body size.
	MaxRequestBodyBytes int64

	// RateLimitPerSecond is the maximum requests per second.
	RateLimitPerSecond int32

	// ClientCAFile is the path to the CA bundle for mTLS client certificate
	// verification. If empty, client certificates are not required.
	ClientCAFile string

	// DeduplicationCacheSize is the size of the auditID LRU cache.
	DeduplicationCacheSize int
}

// NewWebhookIngestor creates a new webhook-based ingestor.
func NewWebhookIngestor(port int32, tlsCert, tlsKey string) *WebhookIngestor {
	return &WebhookIngestor{
		Port:                   port,
		TLSCertFile:            tlsCert,
		TLSKeyFile:             tlsKey,
		MaxRequestBodyBytes:    1048576, // 1MB
		RateLimitPerSecond:     100,
		DeduplicationCacheSize: 10000,
	}
}

// Start begins listening for webhook audit events.
func (w *WebhookIngestor) Start(ctx context.Context) (<-chan auditv1.Event, error) {
	ch := make(chan auditv1.Event, 500)

	dedup := newDeduplicationCache(w.DeduplicationCacheSize)
	limiter := newRateLimiter(int(w.RateLimitPerSecond))

	mux := http.NewServeMux()
	mux.HandleFunc("/", w.handleAuditRequest(ch, dedup, limiter))

	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", w.Port),
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
	}

	// If a client CA is configured, enable mTLS: only clients presenting a
	// certificate signed by this CA (typically the kube-apiserver) are accepted.
	if w.ClientCAFile != "" {
		tlsConfig, err := w.buildMTLSConfig()
		if err != nil {
			return nil, fmt.Errorf("building mTLS config: %w", err)
		}
		server.TLSConfig = tlsConfig
		webhookLog.Info("mTLS enabled", "clientCA", w.ClientCAFile)
	}

	go w.runServer(ctx, server, ch)

	return ch, nil
}

// handleAuditRequest returns an HTTP handler that parses audit EventLists
// and forwards individual events to ch.
func (w *WebhookIngestor) handleAuditRequest(ch chan<- auditv1.Event, dedup *deduplicationCache, limiter *rateLimiter) http.HandlerFunc {
	return func(rw http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			http.Error(rw, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if !limiter.allow() {
			http.Error(rw, "too many requests", http.StatusTooManyRequests)
			return
		}

		body := http.MaxBytesReader(rw, req.Body, w.MaxRequestBodyBytes)
		data, err := io.ReadAll(body)
		if err != nil {
			http.Error(rw, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}

		var eventList auditv1.EventList
		if err := json.Unmarshal(data, &eventList); err != nil {
			http.Error(rw, "invalid audit event payload", http.StatusBadRequest)
			return
		}

		for i := range eventList.Items {
			event := eventList.Items[i]

			auditID := string(event.AuditID)
			if auditID != "" && dedup.seen(auditID) {
				continue
			}

			select {
			case ch <- event:
			default:
				http.Error(rw, "too many requests", http.StatusTooManyRequests)
				return
			}
		}

		rw.WriteHeader(http.StatusOK)
	}
}

// runServer starts the HTTPS server and handles graceful shutdown.
func (w *WebhookIngestor) runServer(ctx context.Context, server *http.Server, ch chan auditv1.Event) {
	defer close(ch)

	errCh := make(chan error, 1)
	go func() {
		webhookLog.Info("starting webhook HTTPS server", "port", w.Port)
		if err := server.ListenAndServeTLS(w.TLSCertFile, w.TLSKeyFile); err != nil && !errors.Is(err, http.ErrServerClosed) {
			webhookLog.Error(err, "webhook server error")
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
	case <-errCh:
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		webhookLog.Error(err, "error shutting down webhook server")
	}
}

// buildMTLSConfig creates a tls.Config that requires and verifies client
// certificates against the CA bundle in ClientCAFile.
func (w *WebhookIngestor) buildMTLSConfig() (*tls.Config, error) {
	caCert, err := os.ReadFile(w.ClientCAFile)
	if err != nil {
		return nil, fmt.Errorf("reading client CA file %s: %w", w.ClientCAFile, err)
	}

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("client CA file %s contains no valid certificates", w.ClientCAFile)
	}

	return &tls.Config{
		ClientAuth: tls.RequireAndVerifyClientCert,
		ClientCAs:  caPool,
		MinVersion: tls.VersionTLS12,
	}, nil
}

// Checkpoint returns an empty position (webhooks are stateless).
func (w *WebhookIngestor) Checkpoint() Position {
	return Position{}
}

// deduplicationCache is a simple bounded cache for deduplicating audit IDs.
type deduplicationCache struct {
	mu      sync.Mutex
	entries map[string]struct{}
	order   []string
	maxSize int
}

func newDeduplicationCache(maxSize int) *deduplicationCache {
	return &deduplicationCache{
		entries: make(map[string]struct{}, maxSize),
		order:   make([]string, 0, maxSize),
		maxSize: maxSize,
	}
}

// seen returns true if the key was already in the cache. Adds it if not.
func (c *deduplicationCache) seen(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.entries[key]; exists {
		return true
	}

	// Evict oldest if at capacity.
	if len(c.order) >= c.maxSize {
		oldest := c.order[0]
		c.order = c.order[1:]
		delete(c.entries, oldest)
	}

	c.entries[key] = struct{}{}
	c.order = append(c.order, key)
	return false
}

// rateLimiter is a simple token bucket rate limiter.
type rateLimiter struct {
	mu         sync.Mutex
	tokens     float64
	maxTokens  float64
	refillRate float64
	lastRefill time.Time
}

func newRateLimiter(perSecond int) *rateLimiter {
	return &rateLimiter{
		tokens:     float64(perSecond),
		maxTokens:  float64(perSecond),
		refillRate: float64(perSecond),
		lastRefill: time.Now(),
	}
}

func (r *rateLimiter) allow() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(r.lastRefill).Seconds()
	r.tokens += elapsed * r.refillRate
	if r.tokens > r.maxTokens {
		r.tokens = r.maxTokens
	}
	r.lastRefill = now

	if r.tokens < 1 {
		return false
	}
	r.tokens--
	return true
}
