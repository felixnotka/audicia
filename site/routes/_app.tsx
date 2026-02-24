import { define } from "../utils.ts";

const SITE_NAME = "Audicia";
const SITE_URL = "https://audicia.io";
const DEFAULT_TITLE =
  "Audicia — Automated RBAC Policy Generation for Kubernetes";
const DEFAULT_DESCRIPTION =
  "Audicia is a Kubernetes Operator that watches audit logs and generates least-privilege RBAC policies. Open source, operator-native, with built-in compliance scoring.";

export default define.page(function App({ Component, url, state }) {
  const title = state?.title || DEFAULT_TITLE;
  const description = state?.description || DEFAULT_DESCRIPTION;
  const canonicalUrl = `${SITE_URL}${url.pathname}`;

  return (
    <html lang="en">
      <head>
        <meta charset="utf-8" />
        <meta name="viewport" content="width=device-width, initial-scale=1.0" />
        <link rel="icon" href="/favicon.ico" />
        <title>{title}</title>
        <link
          rel="preload"
          href="/fonts/inter/Inter-Latin.woff2"
          as="font"
          type="font/woff2"
          crossOrigin="anonymous"
        />
        <link
          rel="preload"
          href="/fonts/jetbrains-mono/JetBrainsMono-Latin.woff2"
          as="font"
          type="font/woff2"
          crossOrigin="anonymous"
        />
        <style
          dangerouslySetInnerHTML={{
            __html: `
          @font-face {
            font-family: "Inter";
            font-style: normal;
            font-weight: 400 700;
            font-display: swap;
            src: url("/fonts/inter/Inter-Latin.woff2") format("woff2");
          }
          @font-face {
            font-family: "JetBrains Mono";
            font-style: normal;
            font-weight: 400 700;
            font-display: swap;
            src: url("/fonts/jetbrains-mono/JetBrainsMono-Latin.woff2") format("woff2");
          }
          html, body {
            background: #0D1B2A;
            color: #E2E8F0;
            font-family: "Inter", ui-sans-serif, system-ui, -apple-system, "Segoe UI", "Helvetica Neue", Arial, sans-serif;
          }
          .navbar { background-color: #0D1B2A; }
        `,
          }}
        />
        <meta name="description" content={description} />
        <meta name="author" content="Felix Notka" />
        <link rel="canonical" href={canonicalUrl} />
        <link
          rel="alternate"
          type="application/rss+xml"
          title="Audicia Blog"
          href="https://audicia.io/blog/feed.xml"
        />

        {/* Open Graph */}
        <meta property="og:site_name" content={SITE_NAME} />
        <meta property="og:title" content={title} />
        <meta property="og:description" content={description} />
        <meta property="og:url" content={canonicalUrl} />
        <meta property="og:type" content="website" />
        <meta property="og:locale" content="en_US" />

        {/* Twitter Card */}
        <meta name="twitter:card" content="summary" />
        <meta name="twitter:title" content={title} />
        <meta name="twitter:description" content={description} />

        <script
          type="application/ld+json"
          // deno-lint-ignore react-no-danger
          dangerouslySetInnerHTML={{
            __html: JSON.stringify({
              "@context": "https://schema.org",
              "@type": "WebSite",
              name: SITE_NAME,
              url: `${SITE_URL}/`,
            }),
          }}
        />
        <script
          // deno-lint-ignore react-no-danger
          dangerouslySetInnerHTML={{
            __html: `
            document.addEventListener('DOMContentLoaded', function() {
              const burger = document.querySelector('.navbar-burger');
              const menu = document.querySelector('.navbar-menu');

              if (burger && menu) {
                burger.addEventListener('click', function(e) {
                  e.stopPropagation();
                  burger.classList.toggle('active');
                  menu.classList.toggle('active');
                });

                const menuItems = document.querySelectorAll('.navbar-menu-item');
                menuItems.forEach(item => {
                  item.addEventListener('click', function() {
                    burger.classList.remove('active');
                    menu.classList.remove('active');
                  });
                });

                menu.addEventListener('click', function(event) {
                  if (event.target === menu) {
                    burger.classList.remove('active');
                    menu.classList.remove('active');
                  }
                });

                document.addEventListener('keydown', function(event) {
                  if (event.key === 'Escape' && menu.classList.contains('active')) {
                    burger.classList.remove('active');
                    menu.classList.remove('active');
                  }
                });
              }
            });
          `,
          }}
        />
      </head>
      <body>
        <a href="#main" className="skip-to-main">Skip to content</a>
        <nav className="navbar" role="navigation" aria-label="Main navigation">
          <a href="/" className="navbar-wordmark">
            <span>A</span>udicia
          </a>
          <button
            type="button"
            className="navbar-burger"
            aria-label="Toggle menu"
          >
            <span className="burger-line"></span>
            <span className="burger-line"></span>
            <span className="burger-line"></span>
          </button>
          <div className="navbar-menu">
            <a className="navbar-menu-item" href="/">Home</a>
            <a className="navbar-menu-item" href="/docs">Docs</a>
            <a className="navbar-menu-item" href="/blog">Blog</a>
            <a
              className="navbar-menu-item"
              href="https://github.com/felixnotka/audicia"
              target="_blank"
              rel="noopener noreferrer"
            >
              GitHub
            </a>
          </div>
        </nav>
        <Component />
        <footer className="footer" role="contentinfo">
          <div className="footer-license">Apache 2.0</div>
          <div className="footer-copyright">&copy; Felix Notka</div>
          <div className="footer-menu">
            <a className="footer-menu-item" href="/docs">Docs</a>
            <a className="footer-menu-item" href="/legal-notice">
              Legal Notice
            </a>
            <a className="footer-menu-item" href="/privacy-policy">
              Privacy Policy
            </a>
            <a
              className="footer-menu-item"
              href="https://github.com/felixnotka/audicia"
              target="_blank"
              rel="noopener noreferrer"
            >
              GitHub
            </a>
            <a
              className="footer-menu-item"
              href="https://github.com/sponsors/felixnotka"
              target="_blank"
              rel="noopener noreferrer"
            >
              Sponsor
            </a>
          </div>
        </footer>
      </body>
    </html>
  );
});
