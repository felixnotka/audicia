export default function Hero() {
  return (
    <section className="hero">
      <div className="hero-inner">
        <div className="hero-content">
          <h1 className="hero-headline">
            Stop Writing RBAC by Hand
          </h1>
          <p className="hero-subheadline">
            Audicia is a <strong>Kubernetes RBAC generator</strong>{" "}
            — an Operator that watches your audit logs and generates
            least-privilege RBAC policies automatically. Open source.
            Operator-native. Never auto-applies.
          </p>
          <div className="hero-ctas">
            <a
              className="hero-cta-primary"
              href="/docs/getting-started/introduction"
            >
              Get Started
            </a>
            <a
              className="hero-cta-secondary"
              href="https://github.com/felixnotka/audicia"
              target="_blank"
              rel="noopener noreferrer"
            >
              View on GitHub
            </a>
          </div>
        </div>
        <div className="hero-terminal">
          <div className="hero-terminal-bar">
            <div className="hero-terminal-dot"></div>
            <div className="hero-terminal-dot"></div>
            <div className="hero-terminal-dot"></div>
          </div>
          <pre className="hero-terminal-body">
            <span className="prompt">$</span>{" "}
            kubectl apply -f audicia-source.yaml{"\n"}
            <span className="output">
              audiciasource.audicia.io/production created
            </span>
            {"\n\n"}
            <span className="prompt">$</span>{" "}
            kubectl get apreport -n my-team -o wide{"\n"}
            <span className="output">
              NAME             SUBJECT   KIND             COMPLIANCE   SCORE   AGE   NEEDED   EXCESS   UNGRANTED   SENSITIVE   AUDIT EVENTS{"\n"}
              report-backend   backend   ServiceAccount   <span className="severity-red">Red</span>          25      5m    2        6        0           true        1500
            </span>
            <span className="hero-terminal-collapsible">
            {"\n\n"}
            <span className="prompt">$</span>{" "}
            kubectl get apreport report-backend -n my-team \{"\n"}
            {"  "}-o jsonpath='{"{"}
            .status.suggestedPolicy.manifests[0]{"}"}'
            {"\n"}
            <span className="keyword">apiVersion</span>: rbac.authorization.k8s.io/v1{"\n"}
            <span className="keyword">kind</span>: Role{"\n"}
            <span className="keyword">metadata</span>:{"\n"}
            {"  "}<span className="keyword">name</span>: <span className="value">suggested-backend-role</span>{"\n"}
            {"  "}<span className="keyword">namespace</span>: my-team{"\n"}
            <span className="keyword">rules</span>:{"\n"}
            {"  "}- <span className="keyword">apiGroups</span>: [""]{"\n"}
            {"    "}<span className="keyword">resources</span>: ["pods", "pods/exec"]{"\n"}
            {"    "}<span className="keyword">verbs</span>: ["create", "get"]
            </span>
          </pre>
        </div>
      </div>
    </section>
  );
}
