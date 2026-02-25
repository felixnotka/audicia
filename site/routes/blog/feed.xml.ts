import { define } from "../../utils.ts";
import { getPosts } from "../../lib/posts.ts";

const SITE_URL = "https://audicia.io";

function escapeXml(str: string): string {
  return str
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&apos;");
}

export const handler = define.handlers({
  async GET() {
    const posts = await getPosts();
    const currentDate = new Date();
    const publishedPosts = posts.filter((p) => p.published_at <= currentDate);

    const lastBuildDate = publishedPosts.length > 0
      ? new Date(publishedPosts[0].published_at).toUTCString()
      : new Date().toUTCString();

    const items = publishedPosts
      .map(
        (post) =>
          `    <item>
      <title>${escapeXml(post.title)}</title>
      <link>${SITE_URL}/blog/${post.slug}</link>
      <guid>${SITE_URL}/blog/${post.slug}</guid>
      <description>${escapeXml(post.description || post.snippet)}</description>
      <pubDate>${new Date(post.published_at).toUTCString()}</pubDate>
    </item>`,
      )
      .join("\n");

    const xml = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:atom="http://www.w3.org/2005/Atom">
  <channel>
    <title>Audicia Blog</title>
    <link>${SITE_URL}/blog</link>
    <description>Technical articles about Kubernetes RBAC, audit logs, and security automation from the Audicia project.</description>
    <language>en-us</language>
    <lastBuildDate>${lastBuildDate}</lastBuildDate>
    <atom:link href="${SITE_URL}/blog/feed.xml" rel="self" type="application/rss+xml" />
${items}
  </channel>
</rss>`;

    return new Response(xml, {
      headers: {
        "content-type": "application/rss+xml; charset=utf-8",
      },
    });
  },
});
