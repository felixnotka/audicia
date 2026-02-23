import { define } from "../utils.ts";

export const handler = define.handlers({
  GET(ctx) {
    ctx.state.title = "Legal Notice — Audicia";
    ctx.state.description = "Legal notice for Audicia.";
    return { data: {} };
  },
});

export default define.page(function LegalNotice() {
  return (
    <div>
      <main id="main" className="page-container full-height home">
        <div className="page-header">
          <h1 className="page-title">Legal Notice</h1>
        </div>

        <div className="page-content">
          <section aria-labelledby="provider">
            <h2 id="provider">Provider pursuant to Section 5 DDG (Germany)</h2>
            <address>
              <div>Felix Notka</div>
              <div>Brünnsteinstraße 4</div>
              <div>85435 Erding, Germany</div>
              <div>
                Email: <a href="mailto:info@audicia.io">info@audicia.io</a>
              </div>
            </address>
          </section>

          <section aria-labelledby="responsible">
            <h2 id="responsible">
              Responsible in accordance with Section 18(2) MStV (Germany)
            </h2>
            <p>Felix Notka, same address as above.</p>
          </section>

          <section aria-labelledby="editorial">
            <h2 id="editorial">Editorial responsibility</h2>
            <p>Felix Notka.</p>
          </section>

          <section aria-labelledby="notes">
            <h2 id="notes">Notes</h2>
            <p>
              This website is a private project providing information about the
              open-source Audicia project. A fast electronic contact is ensured
              via the email address above.
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
