export default function ComplianceScore() {
  return (
    <section className="landing-section">
      <div className="landing-section-inner">
        <h2 className="section-headline">
          How Overprivileged Is Your Cluster?
        </h2>

        <div className="score-cards">
          <div className="score-card" data-severity="red">
            <div className="score-number" data-severity="red">25</div>
            <div className="score-severity" data-severity="red">Red</div>
            <div className="score-label">
              Uses less than half of granted permissions
            </div>
            <div className="score-detail">Unused access to secrets</div>
          </div>

          <div className="score-card" data-severity="yellow">
            <div className="score-number" data-severity="yellow">55</div>
            <div className="score-severity" data-severity="yellow">Yellow</div>
            <div className="score-label">
              Uses 50-80% of granted permissions
            </div>
            <div className="score-detail">Review excess grants</div>
          </div>

          <div className="score-card" data-severity="green">
            <div className="score-number" data-severity="green">92</div>
            <div className="score-severity" data-severity="green">Green</div>
            <div className="score-label">
              Uses more than 80% of granted permissions
            </div>
            <div className="score-detail">Tight. Minimal excess.</div>
          </div>
        </div>

        <div className="score-formula">
          Score = permissions used / permissions granted × 100
        </div>

        <p className="score-explanation">
          Audicia resolves every RoleBinding and ClusterRoleBinding for each
          subject, compares effective RBAC grants against observed audit log
          usage, and scores 0–100. Sensitive excess — unused access to secrets,
          nodes, webhookconfigurations, CRDs, and tokenreviews — is flagged
          separately.
        </p>

        <div className="score-output">
          <pre>
            <span className="prompt">$</span> kubectl get apreport --all-namespaces -o wide{"\n"}
            NAMESPACE   NAME             SUBJECT    KIND             COMPLIANCE   SCORE   AGE   NEEDED   EXCESS   UNGRANTED   SENSITIVE   AUDIT EVENTS{"\n"}
            my-team     report-backend   backend    ServiceAccount   <span className="severity-red">Red</span>          25      5m    2        6        0           true        1500{"\n"}
            my-team     report-frontend  frontend   ServiceAccount   <span className="severity-yellow">Yellow</span>       55      5m    5        4        0           false       3200{"\n"}
            my-team     report-api-gw    api-gw     ServiceAccount   <span className="severity-green">Green</span>        92      5m    11       1        0           false       8400
          </pre>
        </div>
      </div>
    </section>
  );
}
