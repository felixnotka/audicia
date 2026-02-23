export default function OpenSource() {
  return (
    <section className="landing-section">
      <div className="landing-section-inner">
        <h2 className="section-headline">
          Open Source. Apache 2.0. No Catch.
        </h2>

        <p className="opensource-body">
          Everything ships free. The full operator, file and webhook ingestion,
          compliance scoring, the complete Helm chart. There is no paid tier, no
          enterprise edition, no feature gating. I believe security tools should
          be transparent and auditable.
        </p>

        <div className="opensource-ctas">
          <a
            className="hero-cta-primary"
            href="https://github.com/felixnotka/audicia"
            target="_blank"
            rel="noopener noreferrer"
          >
            Star on GitHub
          </a>
          <a
            className="hero-cta-secondary"
            href="https://github.com/felixnotka/audicia"
            target="_blank"
            rel="noopener noreferrer"
          >
            Read the Source
          </a>
          <a
            className="hero-cta-secondary"
            href="https://github.com/felixnotka/audicia/blob/main/CONTRIBUTING.md"
            target="_blank"
            rel="noopener noreferrer"
          >
            Contribute
          </a>
        </div>
      </div>
    </section>
  );
}
