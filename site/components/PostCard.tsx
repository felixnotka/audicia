import type { Post } from "../lib/posts.ts";

export default function PostCard({ post }: { post: Post }) {
  const date = post.published_at instanceof Date
    ? post.published_at
    : new Date(post.published_at);
  const postLink = `/blog/${post.slug}`;

  return (
    <article className="post-card">
      <div className="post-card-header">
        <a
          href={postLink}
          className="post-card-title"
          // deno-lint-ignore react-no-danger
          dangerouslySetInnerHTML={{ __html: post.titleHtml }}
        >
        </a>
        <time className="post-card-date" dateTime={date.toISOString()}>
          {date.toLocaleDateString("en-US", {
            year: "numeric",
            month: "long",
            day: "numeric",
          })}
        </time>
      </div>
      <div className="post-card-content">
        <div
          className="post-card-snippet"
          // deno-lint-ignore react-no-danger
          dangerouslySetInnerHTML={{ __html: post.snippetHtml }}
        >
        </div>
      </div>
      <a href={postLink} className="post-card-read-more">
        read more
      </a>
    </article>
  );
}
