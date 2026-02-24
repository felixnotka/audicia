import { extract } from "@std/front-matter/any";
import { join } from "@std/path";
import { render } from "@deno/gfm";

// Custom renderer for inline content that strips <p> tags
function renderInline(content: string): string {
  const html = render(content);
  // Remove wrapping <p> tags if they exist
  return html.replace(/^<p>/, "").replace(/<\/p>$/, "");
}

const DIRECTORY = "./blog";

export interface Post {
  slug: string;
  title: string;
  titleHtml: string;
  seo_title?: string;
  published_at: Date;
  content: string;
  snippet: string;
  snippetHtml: string;
  description?: string;
  cover_image?: string;
  cover_image_alt?: string;
}

export interface FrontMatter {
  title: string;
  seo_title?: string;
  published_at: string;
  snippet: string;
  description?: string;
  cover_image?: string;
  cover_image_alt?: string;
}

export async function getPosts(): Promise<Post[]> {
  const entries = Deno.readDir(DIRECTORY);
  const promises: Promise<Post | null>[] = [];

  for await (const entry of entries) {
    if (!entry.isFile || !entry.name.endsWith(".md")) continue;

    const slug = entry.name.slice(0, -".md".length);
    promises.push(getPost(slug));
  }

  const posts = (await Promise.all(promises)).filter(
    (post): post is Post => post !== null,
  );

  posts.sort((a, b) => b.published_at.getTime() - a.published_at.getTime());

  return posts;
}

export async function getPost(slug: string): Promise<Post | null> {
  try {
    const filePath = join(DIRECTORY, `${slug}.md`);
    const text = await Deno.readTextFile(filePath);
    const { attrs, body } = extract<FrontMatter>(text);

    const publishedAt = new Date(attrs.published_at);
    if (Number.isNaN(publishedAt.getTime())) {
      throw new Error(
        `Invalid "published_at" date in front matter for slug "${slug}": "${attrs.published_at}"`,
      );
    }

    return {
      slug,
      title: attrs.title,
      titleHtml: renderInline(attrs.title),
      seo_title: attrs.seo_title,
      published_at: publishedAt,
      content: body,
      snippet: attrs.snippet,
      snippetHtml: renderInline(attrs.snippet),
      description: attrs.description,
      cover_image: attrs.cover_image,
      cover_image_alt: attrs.cover_image_alt,
    };
  } catch (error) {
    console.error(`Failed to load post "${slug}":`, error);
    return null;
  }
}
