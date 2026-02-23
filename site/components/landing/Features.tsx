export default function Features() {
  const features = [
    {
      icon: "\u21C4",
      title: "Continuous Ingestion",
      body:
        "Tail audit log files with checkpoint/resume and inode-based rotation detection. Or receive real-time events via HTTPS webhook with TLS and mTLS. Both modes run simultaneously.",
    },
    {
      icon: "\u2699",
      title: "Smart Normalization",
      body:
        "Parses system:serviceaccount:ns:name into structured identities. Migrates deprecated API groups. Concatenates subresources. Handles non-resource URLs.",
    },
    {
      icon: "\u2630",
      title: "Noise Filtering",
      body:
        "Ordered allow/deny chains with first-match-wins semantics. Drop system:node and system:kube noise while preserving service account events.",
    },
    {
      icon: "\u2318",
      title: "Policy Strategy Knobs",
      body:
        "Control scope (namespace vs cluster), verb merging (smart collapses get+list+watch), and wildcard generation (forbidden by default).",
    },
    {
      icon: "\u2714",
      title: "Compliance Scoring",
      body:
        "Resolves all RoleBindings and ClusterRoleBindings for each subject. Compares effective RBAC grants against observed usage. Scores 0\u2013100 with Green, Yellow, and Red.",
    },
    {
      icon: "\u26A0",
      title: "Sensitive Excess Detection",
      body:
        "Flags unused grants on high-risk resources: secrets, nodes, webhookconfigurations, CRDs, tokenreviews, and more.",
    },
  ];

  return (
    <section className="landing-section">
      <div className="landing-section-inner">
        <h2 className="section-headline">
          Everything You Need, Nothing You Don't
        </h2>

        <div className="feature-grid">
          {features.map((feature) => (
            <div key={feature.title} className="feature-card">
              <div className="feature-card-icon">{feature.icon}</div>
              <h3 className="feature-card-title">{feature.title}</h3>
              <p className="feature-card-body">{feature.body}</p>
            </div>
          ))}
        </div>
      </div>
    </section>
  );
}
