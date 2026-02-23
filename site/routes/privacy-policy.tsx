import { define } from "../utils.ts";

export const handler = define.handlers({
  GET(ctx) {
    ctx.state.title = "Privacy Policy — Audicia";
    ctx.state.description = "Privacy policy for Audicia.";
    return { data: {} };
  },
});

export default define.page(function PrivacyPolicy() {
  return (
    <div>
      <main id="main" className="page-container home">
        <div className="page-header">
          <h1 className="page-title">Privacy Policy</h1>
        </div>

        <div className="page-content">
          <section aria-labelledby="controller">
            <h2 id="controller">Controller (Art. 4(7) GDPR)</h2>
            <address>
              <div>Felix Notka</div>
              <div>Brünnsteinstraße 4</div>
              <div>85435 Erding, Germany</div>
              <div>
                Email: <a href="mailto:info@audicia.io">info@audicia.io</a>
              </div>
            </address>
          </section>

          <section aria-labelledby="hosting">
            <h2 id="hosting">1) Hosting and Domain</h2>
            <p>
              This website runs on my self-managed cluster in Germany. I operate
              my infrastructure with providers <strong>Strato AG</strong> and/or
              {" "}
              <strong>Hetzner Online GmbH</strong>{" "}
              (Germany). The domain is registered with{" "}
              <strong>Cloudflare</strong>. These providers act as processors on
              my behalf (Art. 28 GDPR). There are{" "}
              <strong>no third-country transfers</strong>.
            </p>
          </section>

          <section aria-labelledby="logs">
            <h2 id="logs">2) Server Log Files</h2>
            <p>
              When you access this site, I process the following data to ensure
              technical operation and security (legitimate interests, Art.
              6(1)(f) GDPR):
            </p>
            <ul>
              <li>IP address</li>
              <li>Date and time of request</li>
              <li>Requested URL and HTTP status code</li>
              <li>Referrer URL (if sent)</li>
              <li>User agent (browser, operating system)</li>
            </ul>
            <p>
              Log data is written <strong>inside the container</strong>{" "}
              as access logs and retained for{" "}
              <strong>28 days</strong>, then deleted. If required for
              operations, I may visualize logs in my{" "}
              <strong>self-hosted Grafana stack</strong>{" "}
              (no third-party SaaS; data remains in Germany).
            </p>
          </section>

          <section aria-labelledby="cookies">
            <h2 id="cookies">3) Cookies, Local Storage, Third-Party Content</h2>
            <p>
              I do <strong>not</strong>{" "}
              use analytics, tracking, marketing cookies, or comparable
              technologies. All fonts and assets are served{" "}
              <strong>locally</strong>. I do <strong>not</strong>{" "}
              embed third-party content (e.g., YouTube, Maps, social widgets).
              Therefore, no consent banner is required.
            </p>
          </section>

          <section aria-labelledby="contact">
            <h2 id="contact">4) Contact via Email</h2>
            <p>
              If you contact me by email, I process your message, email address,
              and any information you provide in order to handle your inquiry.
              <br />
              <strong>Legal basis:</strong>{" "}
              Art. 6(1)(f) GDPR (legitimate interest in responding); if the
              inquiry aims at entering into a contract, additionally Art.
              6(1)(b) GDPR.
              <br />
              <strong>Retention:</strong>{" "}
              I keep correspondence as business records where applicable
              (generally up to 6 years under German law) and otherwise delete it
              when the matter is resolved and no further retention is required.
            </p>
          </section>

          <section aria-labelledby="recipients">
            <h2 id="recipients">5) Recipients</h2>
            <p>
              For operations, data may be processed by the hosting providers
              named above as <em>processors</em> bound by my instructions. I do
              {" "}
              <strong>not</strong> sell personal data.
            </p>
          </section>

          <section aria-labelledby="rights">
            <h2 id="rights">6) Your Rights (Art. 15–21 GDPR)</h2>
            <p>
              You have the rights to access, rectification, erasure,
              restriction, data portability, and to object to processing based
              on Art. 6(1)(f) GDPR. You may also withdraw consent at any time
              with effect for the future (where applicable).
            </p>
          </section>

          <section aria-labelledby="authority">
            <h2 id="authority">7) Supervisory Authority</h2>
            <p>
              You may lodge a complaint with a data protection authority, in
              particular with the authority competent for your habitual
              residence or for my seat. Competent authority for my seat
              (Bavaria):
              <br />
              <strong>
                Bayerisches Landesamt für Datenschutzaufsicht (BayLDA)
              </strong>.
            </p>
          </section>

          <section aria-labelledby="security">
            <h2 id="security">8) Security</h2>
            <p>
              I use transport encryption (TLS) when you access this website and
              apply technical and organizational measures appropriate to the
              risk.
            </p>
          </section>

          <section aria-labelledby="changes">
            <h2 id="changes">9) Changes to this Notice</h2>
            <p>
              I may update this Privacy Policy to reflect changes in technology
              or law. The current version is published on this page.
            </p>
          </section>

          <p className="last-updated">
            <em>Last updated: 22 February 2026</em>
          </p>
        </div>
      </main>
    </div>
  );
});
