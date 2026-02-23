import { Head } from "fresh/runtime";
import { HttpError } from "fresh";
import { define } from "../../utils.ts";
import { getPost } from "../../lib/posts.ts";
import { CSS, render } from "@deno/gfm";

export const handler = define.handlers({
  async GET(ctx) {
    const post = await getPost(ctx.params.slug);

    if (!post) {
      throw new HttpError(404, "Post not found");
    }

    ctx.state.title = `${post.title} — Audicia Blog`;
    ctx.state.description = post.snippet;

    return { data: { post } };
  },
});

export default define.page<typeof handler>(function Post(props) {
  const { post } = props.data;
  const postUrl = `https://audicia.io/blog/${post.slug}`;
  const publishedIso = new Date(post.published_at).toISOString();

  return (
    <div>
      <Head>
        <meta property="og:type" content="article" />
        <meta property="og:title" content={post.title} />
        <meta property="og:description" content={post.snippet} />
        <meta property="og:url" content={postUrl} />
        <meta property="article:published_time" content={publishedIso} />
        <meta property="article:author" content="Felix Notka" />
        <meta name="twitter:card" content="summary" />
        <meta name="twitter:title" content={post.title} />
        <meta name="twitter:description" content={post.snippet} />
        <script
          type="application/ld+json"
          // deno-lint-ignore react-no-danger
          dangerouslySetInnerHTML={{
            __html: JSON.stringify({
              "@context": "https://schema.org",
              "@type": "BlogPosting",
              headline: post.title,
              description: post.snippet,
              datePublished: publishedIso,
              url: postUrl,
              author: {
                "@type": "Person",
                name: "Felix Notka",
              },
              publisher: {
                "@type": "Organization",
                name: "Audicia",
                url: "https://audicia.io/",
              },
            }),
          }}
        />
      </Head>
      <style
        dangerouslySetInnerHTML={{ __html: CSS }}
      />
      <main id="main" className="post-container">
        <div className="post-header">
          <h1
            className="post-title"
            // deno-lint-ignore react-no-danger
            dangerouslySetInnerHTML={{ __html: post.titleHtml }}
          >
          </h1>
          <div className="post-metadata">
            <time className="post-date" dateTime={publishedIso}>
              {new Date(post.published_at).toLocaleDateString("en-us", {
                year: "numeric",
                month: "long",
                day: "numeric",
              })}
            </time>
          </div>
        </div>
        <div className="post-content">
          <div className="post-hero-spacer">
            <div className="post-hero-spacer-decoration"></div>
            <div className="post-hero-spacer-decoration"></div>
            <div className="post-hero-spacer-decoration"></div>
          </div>
          {post.cover_image && (
            <div className="post-hero">
              <img
                src={post.cover_image}
                alt={post.cover_image_alt || post.title}
              />
            </div>
          )}
          <div
            className="post-body markdown-body"
            // deno-lint-ignore react-no-danger
            dangerouslySetInnerHTML={{ __html: render(post.content) }}
          />
        </div>
      </main>
    </div>
  );
});
