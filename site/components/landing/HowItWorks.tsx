export default function HowItWorks() {
  return (
    <section className="landing-section">
      <div className="landing-section-inner">
        <h2 className="section-headline">From Audit Logs to Least Privilege</h2>

        <div className="pipeline-steps">
          <div className="pipeline-step">
            <div className="pipeline-step-number">Step 01</div>
            <h3 className="pipeline-step-title">
              Point Audicia at your audit log
            </h3>
            <p className="pipeline-step-body">
              Create an AudiciaSource custom resource. Audicia starts tailing the
              log file or receiving real-time events via webhook. Both modes
              support checkpoint/resume — no data is lost on restart.
            </p>
          </div>

          <div className="pipeline-step">
            <div className="pipeline-step-number">Step 02</div>
            <h3 className="pipeline-step-title">
              Audicia observes access patterns
            </h3>
            <p className="pipeline-step-body">
              The pipeline processes both allowed (200) and denied (403)
              requests. It normalizes subjects, handles subresources, and
              migrates deprecated API groups. Configurable filters drop system
              noise while keeping your workloads.
            </p>
          </div>

          <div className="pipeline-step">
            <div className="pipeline-step-number">Step 03</div>
            <h3 className="pipeline-step-title">
              Get a compliance-scored policy report
            </h3>
            <p className="pipeline-step-body">
              An AudiciaPolicyReport CR appears for each subject. It contains
              observed rules, ready-to-apply Role and RoleBinding YAML, and a
              compliance score comparing observed usage against effective RBAC
              grants.
            </p>
          </div>

          <div className="pipeline-step">
            <div className="pipeline-step-number">Step 04</div>
            <h3 className="pipeline-step-title">Review and apply</h3>
            <p className="pipeline-step-body">
              Use kubectl apply, commit to Git for ArgoCD/Flux, or wait for the
              upcoming dashboard. Audicia generates recommendations. A human or
              reviewed pipeline applies them. Never auto-applies.
            </p>
          </div>
        </div>
      </div>
    </section>
  );
}
