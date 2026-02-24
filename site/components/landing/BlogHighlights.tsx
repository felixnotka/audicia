const posts = [
  {
    title: "Introducing Audicia: Stop Writing RBAC by Hand",
    snippet:
      "Why I built Audicia and how it generates least-privilege RBAC from audit logs.",
    href: "/blog/introducing-audicia",
  },
  {
    title: "How the Pipeline Turns Audit Logs into RBAC Policies",
    snippet:
      "Inside the six-stage pipeline: ingest, filter, normalize, aggregate, strategy, report.",
    href: "/blog/how-audicia-processes-audit-logs",
  },
  {
    title: "Understanding Compliance Scores: Red, Yellow, Green",
    snippet:
      "How Audicia scores every service account and what to do about a Red rating.",
    href: "/blog/understanding-compliance-scores",
  },
];

export default function BlogHighlights() {
  return (
    <section className="landing-section">
      <div className="landing-section-inner">
        <h2 className="section-headline">From the Blog</h2>
        <div className="blog-highlights">
          {posts.map((post) => (
            <a key={post.href} href={post.href} className="blog-highlight-card">
              <h3 className="blog-highlight-title">{post.title}</h3>
              <p className="blog-highlight-snippet">{post.snippet}</p>
            </a>
          ))}
        </div>
      </div>
    </section>
  );
}
