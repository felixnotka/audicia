export default function Comparison() {
  return (
    <section className="landing-section">
      <div className="landing-section-inner">
        <h2 className="section-headline">Not Another Scanner</h2>
        <p className="section-subheadline">
          Kubernetes security tools fall into three categories: scanners (find
          problems), enforcers (block violations), and generators (create correct
          policies). Audicia is a continuous, operator-native policy generator.
        </p>

        <div className="comparison-quadrant-wrapper">
          <span className="comparison-axis-label comparison-axis-top">
            Static Analysis
          </span>
          <span className="comparison-axis-label comparison-axis-bottom">
            Runtime Analysis
          </span>
          <span className="comparison-axis-label comparison-axis-left">
            Policy Scanning
          </span>
          <span className="comparison-axis-label comparison-axis-right">
            Policy Generation
          </span>

          <div className="comparison-quadrant">
            <div className="comparison-quadrant-cell comparison-cell-tl">
              <span className="comparison-tool">Trivy</span>
              <span className="comparison-tool">KubeAudit</span>
              <span className="comparison-tool">KubeLinter</span>
            </div>
            <div className="comparison-quadrant-cell comparison-cell-tr">
              <span className="comparison-tool">OPA/Gatekeeper</span>
            </div>
            <div className="comparison-quadrant-cell comparison-cell-bl">
              <span className="comparison-tool">audit2rbac</span>
            </div>
            <div className="comparison-quadrant-cell comparison-cell-br">
              <span className="comparison-tool comparison-tool-highlight">
                Audicia
              </span>
            </div>
          </div>
        </div>

        <ul className="comparison-bullets">
          <li>
            Only Kubernetes Operator for continuous RBAC generation
          </li>
          <li>
            Stateful processing with checkpoint/resume — not re-read-everything
          </li>
          <li>
            CRD output for GitOps integration — not raw YAML dumps
          </li>
          <li>Compliance scoring built-in — not a separate tool</li>
        </ul>
      </div>
    </section>
  );
}
