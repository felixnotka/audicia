package main

import (
	"context"
	"fmt"
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

	if err := operator.Start(ctx, buildInfo, config); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
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
