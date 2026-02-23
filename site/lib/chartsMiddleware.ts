import { join } from "@std/path";
import { define } from "../utils.ts";

const CHARTS_DIR = "./static/charts";

export const chartsMiddleware = define.middleware(async (ctx) => {
  const host = ctx.req.headers.get("host") || "";

  if (!host.startsWith("charts.")) {
    return ctx.next();
  }

  const path = ctx.url.pathname;

  if (path === "/" || path === "/index.yaml") {
    try {
      const content = await Deno.readTextFile(join(CHARTS_DIR, "index.yaml"));
      return new Response(content, {
        headers: {
          "content-type": "application/x-yaml",
          "cache-control": "public, max-age=300",
        },
      });
    } catch {
      return new Response("index.yaml not found", { status: 404 });
    }
  }

  if (path.endsWith(".tgz")) {
    const filename = path.split("/").pop()!;
    try {
      const file = await Deno.readFile(join(CHARTS_DIR, filename));
      return new Response(file, {
        headers: {
          "content-type": "application/gzip",
          "content-disposition": `attachment; filename="${filename}"`,
          "cache-control": "public, max-age=86400",
        },
      });
    } catch {
      return new Response("Chart not found", { status: 404 });
    }
  }

  return new Response("Not found", { status: 404 });
});
