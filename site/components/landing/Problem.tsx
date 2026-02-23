export default function Problem() {
  return (
    <section className="landing-section">
      <div className="landing-section-inner">
        <h2 className="section-headline">RBAC Is Broken in Practice</h2>

        <div className="problem-grid">
          <div className="problem-card">
            <h3 className="problem-card-title">The 403 Cycle</h3>
            <p className="problem-card-body">
              Your service returns 403 Forbidden. Someone escalates to
              cluster-admin "just to unblock." It's never reverted. Multiply this
              across every microservice, every team, every namespace. Your
              cluster's RBAC is now a pile of overprivileged bindings that nobody
              understands.
            </p>
          </div>

          <div className="problem-card">
            <h3 className="problem-card-title">Silent Drift</h3>
            <p className="problem-card-body">
              Permissions accumulate over time. Service accounts gain access to
              secrets, nodes, and webhooks they'll never touch. Nobody audits
              them because auditing RBAC manually is a full-time job that nobody
              signed up for.
            </p>
          </div>

          <div className="problem-card">
            <h3 className="problem-card-title">Compliance Theater</h3>
            <p className="problem-card-body">
              Auditors ask for evidence of least-privilege. You produce
              spreadsheets, screenshots, and promises. The evidence is stale by
              the time the audit is over. Everyone knows it, but the manual
              alternative is worse.
            </p>
          </div>
        </div>
      </div>
    </section>
  );
}
