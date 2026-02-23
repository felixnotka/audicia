package operator

import "time"

// Config holds the operator configuration, loaded from environment variables.
type Config struct {
	// MetricsBindAddress is the address the metrics endpoint binds to.
	MetricsBindAddress string `env:"METRICS_BIND_ADDRESS" envDefault:":8080"`

	// HealthProbeBindAddress is the address the health probe endpoint binds to.
	HealthProbeBindAddress string `env:"HEALTH_PROBE_BIND_ADDRESS" envDefault:":8081"`

	// LeaderElectionEnabled enables leader election for the controller manager.
	LeaderElectionEnabled bool `env:"LEADER_ELECTION_ENABLED" envDefault:"true"`

	// LeaderElectionID is the name of the leader election resource.
	LeaderElectionID string `env:"LEADER_ELECTION_ID" envDefault:"audicia-operator-lock"`

	// LeaderElectionNamespace is the namespace for the leader election resource.
	LeaderElectionNamespace string `env:"LEADER_ELECTION_NAMESPACE" envDefault:"audicia-system"`

	// ConcurrentReconciles is the number of concurrent reconcile loops.
	ConcurrentReconciles int `env:"CONCURRENT_RECONCILES" envDefault:"1"`

	// LogLevel is the log verbosity (0=info, 1=debug, 2=trace).
	LogLevel int `env:"LOG_LEVEL" envDefault:"0"`

	// SyncPeriod is the minimum interval between full reconciliations.
	SyncPeriod time.Duration `env:"SYNC_PERIOD" envDefault:"10m"`
}
