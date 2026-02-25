const faqs = [
  {
    q: "How long until I see results?",
    a: "About five minutes. Install the Helm chart, create an AudiciaSource pointing at your audit log, and Audicia starts generating policy reports as soon as events flow in. The more traffic your cluster has, the faster the reports fill out.",
  },
  {
    q: "Will Audicia automatically apply RBAC policies?",
    a: "No. Audicia never auto-applies policies. It generates recommendations as Kubernetes resources. A human or a reviewed CI pipeline decides what to apply. Automated privilege escalation is a security anti-pattern.",
  },
  {
    q: "What does a compliance score mean?",
    a: "The score measures how much of a subject's granted RBAC permissions are actually used. A score of 100 means tight permissions. A score of 25 means 75% of the granted access is unused. Scores map to Green (76–100), Yellow (34–75), and Red (0–33) severity bands.",
  },
  {
    q: "What compliance frameworks does it map to?",
    a: "AudiciaPolicyReports map to SOC 2 CC6.1 (logical access security), ISO 27001 A.8.3 (access restriction), PCI DSS Requirement 7 (need-to-know access), and NIST AC-6 (least privilege).",
  },
  {
    q: "What does Audicia need to run?",
    a: "A Kubernetes cluster (1.32+) with audit logging enabled. Audicia supports three ingestion modes: tailing the audit log file, receiving events via a webhook backend, or consuming from a cloud message bus (Azure Event Hub, AWS CloudWatch, GCP Pub/Sub). The Helm chart handles the rest.",
  },
  {
    q: "Is there a paid version?",
    a: "No. Audicia is Apache 2.0 licensed. The full operator, all three ingestion modes (file, webhook, cloud), compliance scoring, and the complete Helm chart ship free. There is no paid tier, no enterprise edition, no feature gating.",
  },
];

export default function Faq() {
  return (
    <section className="landing-section">
      <div className="landing-section-inner">
        <h2 className="section-headline">Frequently Asked Questions</h2>

        <div className="faq-list">
          {faqs.map((faq) => (
            <details key={faq.q} className="faq-item">
              <summary className="faq-question">{faq.q}</summary>
              <p className="faq-answer">{faq.a}</p>
            </details>
          ))}
        </div>
      </div>
    </section>
  );
}
