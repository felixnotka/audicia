import { define } from "../utils.ts";
import { getPosts } from "../lib/posts.ts";

const SITE_URL = "https://audicia.io";

export const handler = define.handlers({
  async GET() {
    const posts = await getPosts();
    const currentDate = new Date();
    const publishedPosts = posts.filter((p) => p.published_at <= currentDate);

    const staticPages = [
      { loc: "/", changefreq: "weekly", priority: "1.0" },
      { loc: "/blog", changefreq: "weekly", priority: "0.8" },
      { loc: "/legal-notice", changefreq: "yearly", priority: "0.3" },
      { loc: "/privacy-policy", changefreq: "yearly", priority: "0.3" },
    ];

    const urls = staticPages
      .map(
        (page) =>
          `  <url>
    <loc>${SITE_URL}${page.loc}</loc>
    <changefreq>${page.changefreq}</changefreq>
    <priority>${page.priority}</priority>
  </url>`,
      )
      .concat(
        publishedPosts.map(
          (post) =>
            `  <url>
    <loc>${SITE_URL}/blog/${post.slug}</loc>
    <lastmod>${
              new Date(post.published_at).toISOString().split("T")[0]
            }</lastmod>
    <changefreq>monthly</changefreq>
    <priority>0.6</priority>
  </url>`,
        ),
      )
      .join("\n");

    const xml = `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
${urls}
</urlset>`;

    return new Response(xml, {
      headers: {
        "content-type": "application/xml; charset=utf-8",
      },
    });
  },
});
