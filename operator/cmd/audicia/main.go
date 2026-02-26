package main

import (
	"context"
	"fmt"
	"math"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/felixnotka/audicia/operator/pkg/operator"
)

// Build-time variables injected via ldflags.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Printf("audicia-operator %s (commit: %s, built: %s)\n", version, commit, date)
		fmt.Println("Author: Felix Notka <https://github.com/felixnotka>")
		os.Exit(0)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	buildInfo := operator.BuildInfo{
		Version: version,
		Commit:  commit,
		Date:    date,
	}

	config := loadConfig()

	maxRetries := envInt("STARTUP_MAX_RETRIES", 5)
	if err := startWithRetry(ctx, buildInfo, config, maxRetries); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// startWithRetry wraps operator.Start with exponential backoff. This handles
// transient API server connectivity issues (e.g. CNI not ready, control plane
// restart) without relying on Kubernetes' slow CrashLoopBackOff (10s → 5min).
func startWithRetry(ctx context.Context, buildInfo operator.BuildInfo, config operator.Config, maxRetries int) error {
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			delay := time.Duration(math.Min(float64(time.Second)*math.Pow(2, float64(attempt)), float64(60*time.Second)))
			_, _ = fmt.Fprintf(os.Stderr, "startup failed (attempt %d/%d), retrying in %s: %v\n",
				attempt, maxRetries, delay, lastErr)

			select {
			case <-ctx.Done():
				return fmt.Errorf("interrupted during startup retry: %w", lastErr)
			case <-time.After(delay):
			}
		}

		lastErr = operator.Start(ctx, buildInfo, config)
		if lastErr == nil {
			return nil
		}

		// If the context was cancelled (SIGTERM/SIGINT), don't retry.
		if ctx.Err() != nil {
			return lastErr
		}
	}
	return fmt.Errorf("operator failed after %d attempts: %w", maxRetries+1, lastErr)
}

// loadConfig reads operator configuration from environment variables with defaults.
func loadConfig() operator.Config {
	return operator.Config{
		MetricsBindAddress:      envString("METRICS_BIND_ADDRESS", ":8080"),
		HealthProbeBindAddress:  envString("HEALTH_PROBE_BIND_ADDRESS", ":8081"),
		LeaderElectionEnabled:   envBool("LEADER_ELECTION_ENABLED", true),
		LeaderElectionID:        envString("LEADER_ELECTION_ID", "audicia-operator-lock"),
		LeaderElectionNamespace: envString("LEADER_ELECTION_NAMESPACE", "audicia-system"),
		ConcurrentReconciles:    envInt("CONCURRENT_RECONCILES", 1),
		LogLevel:                envInt("LOG_LEVEL", 0),
		SyncPeriod:              envDuration("SYNC_PERIOD", 10*time.Minute),
	}
}

func envString(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func envBool(key string, defaultVal bool) bool {
	if v := os.Getenv(key); v != "" {
		b, err := strconv.ParseBool(v)
		if err == nil {
			return b
		}
	}
	return defaultVal
}

func envInt(key string, defaultVal int) int {
	if v := os.Getenv(key); v != "" {
		i, err := strconv.Atoi(v)
		if err == nil {
			return i
		}
	}
	return defaultVal
}

func envDuration(key string, defaultVal time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		d, err := time.ParseDuration(v)
		if err == nil {
			return d
		}
	}
	return defaultVal
}
