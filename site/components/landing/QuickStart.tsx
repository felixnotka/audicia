export default function QuickStart() {
  return (
    <section className="landing-section">
      <div className="landing-section-inner">
        <h2 className="section-headline">
          Three Commands to Least Privilege
        </h2>
        <p className="section-subheadline">
          Audicia installs via Helm and works with standard kube-apiserver audit
          log paths out of the box.
        </p>

        <div className="quickstart-terminal">
          <div className="quickstart-terminal-bar">
            <div className="quickstart-terminal-dot"></div>
            <div className="quickstart-terminal-dot"></div>
            <div className="quickstart-terminal-dot"></div>
          </div>
          <pre className="quickstart-terminal-body">
            <span className="comment"># Install</span>{"\n"}
            <span className="prompt">$</span> helm install audicia ./deploy/helm -n audicia-system --create-namespace{"\n"}
            {"\n"}
            <span className="comment"># Configure</span>{"\n"}
            <span className="prompt">$</span> kubectl apply -f audicia-source.yaml{"\n"}
            {"\n"}
            <span className="comment"># Check</span>{"\n"}
            <span className="prompt">$</span> kubectl get apreport --all-namespaces
          </pre>
        </div>

        <div className="quickstart-cta">
          <a
            className="hero-cta-primary"
            href="/docs/getting-started/introduction"
          >
            Read the Full Guide
          </a>
        </div>
      </div>
    </section>
  );
}
