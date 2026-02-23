export default function Compliance() {
  return (
    <section className="landing-section">
      <div className="landing-section-inner">
        <h2 className="section-headline">Always Audit-Ready</h2>

        <div className="compliance-grid">
          <div className="compliance-column compliance-column-before">
            <h3 className="compliance-column-title">Before Audicia</h3>
            <ul>
              <li>Quarterly compliance scrambles</li>
              <li>Manual spreadsheets with stale data</li>
              <li>"Trust us, we follow least-privilege"</li>
              <li>Evidence: screenshots of kubectl output</li>
            </ul>
          </div>

          <div className="compliance-column compliance-column-after">
            <h3 className="compliance-column-title">With Audicia</h3>
            <ul>
              <li>Continuous compliance evidence as Kubernetes CRDs</li>
              <li>Each report is timestamped, diffable, and version-controlled</li>
              <li>
                Maps to SOC 2, ISO 27001, PCI DSS, and NIST controls
              </li>
              <li>
                Evidence: AudiciaPolicyReport showing score, excess grants,
                and sensitive resources
              </li>
            </ul>
          </div>
        </div>

        <div className="compliance-frameworks">
          <div className="compliance-framework">
            <div className="compliance-framework-name">SOC 2 CC6.1</div>
            <div className="compliance-framework-desc">
              Logical access security
            </div>
          </div>
          <div className="compliance-framework">
            <div className="compliance-framework-name">ISO 27001 A.8.3</div>
            <div className="compliance-framework-desc">
              Information access restriction
            </div>
          </div>
          <div className="compliance-framework">
            <div className="compliance-framework-name">PCI DSS Req 7</div>
            <div className="compliance-framework-desc">
              Restrict access to need-to-know
            </div>
          </div>
          <div className="compliance-framework">
            <div className="compliance-framework-name">NIST AC-6</div>
            <div className="compliance-framework-desc">Least privilege</div>
          </div>
        </div>
      </div>
    </section>
  );
}
