import { App, staticFiles } from "fresh";
import { chartsMiddleware } from "./lib/chartsMiddleware.ts";
import { loggingMiddleware } from "./lib/loggingMiddleware.ts";

const notFoundHtml = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1.0" />
  <title>Page Not Found — Audicia</title>
  <link rel="icon" href="/favicon.ico" />
  <style>
    @font-face {
      font-family: "Inter";
      font-style: normal;
      font-weight: 400 700;
      font-display: swap;
      src: url("/fonts/inter/Inter-Latin.woff2") format("woff2");
    }
    * { margin: 0; padding: 0; box-sizing: border-box; }
    html, body {
      background: #0D1B2A;
      color: #E2E8F0;
      font-family: "Inter", ui-sans-serif, system-ui, -apple-system, sans-serif;
      min-height: 100vh;
    }
    .not-found {
      display: flex;
      flex-direction: column;
      align-items: center;
      justify-content: center;
      min-height: 80vh;
      text-align: center;
      padding: 2rem;
    }
    .not-found-code {
      font-size: 6rem;
      font-weight: 700;
      color: #FF6B35;
      line-height: 1;
    }
    .not-found-title {
      font-size: 1.5rem;
      font-weight: 600;
      margin: 1rem 0 0.5rem;
    }
    .not-found-message {
      color: #94A3B8;
      margin-bottom: 2rem;
      max-width: 28rem;
    }
    .not-found-links {
      display: flex;
      gap: 1rem;
      flex-wrap: wrap;
      justify-content: center;
    }
    .not-found-links a {
      color: #FF6B35;
      text-decoration: none;
      padding: 0.5rem 1rem;
      border: 1px solid #FF6B35;
      border-radius: 6px;
      transition: background 0.15s, color 0.15s;
    }
    .not-found-links a:hover {
      background: #FF6B35;
      color: #0D1B2A;
    }
  </style>
</head>
<body>
  <div class="not-found">
    <div class="not-found-code">404</div>
    <h1 class="not-found-title">Page Not Found</h1>
    <p class="not-found-message">
      The page you're looking for doesn't exist or has been moved.
    </p>
    <div class="not-found-links">
      <a href="/">Home</a>
      <a href="/docs">Docs</a>
      <a href="/blog">Blog</a>
      <a href="https://github.com/felixnotka/audicia">GitHub</a>
    </div>
  </div>
</body>
</html>`;

export const app = new App()
  .use(chartsMiddleware)
  .use(staticFiles())
  .use(loggingMiddleware)
  .onError("*", (ctx) => {
    console.log(`Error: ${ctx.error}`);
    if (ctx.error?.status === 404) {
      return new Response(notFoundHtml, {
        status: 404,
        headers: { "content-type": "text/html; charset=utf-8" },
      });
    }
    return new Response("Oops!", { status: 500 });
  })
  .notFound(() => {
    return new Response(notFoundHtml, {
      status: 404,
      headers: { "content-type": "text/html; charset=utf-8" },
    });
  });

app.fsRoutes();
