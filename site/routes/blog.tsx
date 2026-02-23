import { define } from "../utils.ts";
import { getPosts, Post } from "../lib/posts.ts";
import PostCard from "../components/PostCard.tsx";

export const handler = define.handlers({
  async GET(ctx) {
    const allPosts = await getPosts();
    const currentDate = new Date();
    const posts = allPosts.filter((post) => post.published_at <= currentDate);
    ctx.state.title = "Blog — Audicia";
    ctx.state.description =
      "Technical articles about Kubernetes RBAC, audit logs, and security automation.";
    return { data: { posts } };
  },
});

export default define.page<typeof handler>(function Blog(props) {
  const { posts } = props.data;

  const postsByMonth = posts.reduce((groups, post) => {
    const date = new Date(post.published_at);
    const monthKey = `${date.getFullYear()}-${
      String(date.getMonth() + 1).padStart(2, "0")
    }`;
    const monthName = date.toLocaleDateString("en-US", {
      year: "numeric",
      month: "long",
    });

    if (!groups[monthKey]) {
      groups[monthKey] = {
        monthName,
        posts: [],
      };
    }
    groups[monthKey].posts.push(post);
    return groups;
  }, {} as Record<string, { monthName: string; posts: Post[] }>);

  const sortedMonths = Object.entries(postsByMonth)
    .sort(([a], [b]) => b.localeCompare(a));

  return (
    <main id="main" className="page-container">
      <div className="page-header">
        <h1 className="page-title">Latest Posts</h1>
      </div>
      <div className="page-content">
        {sortedMonths.map(([monthKey, { monthName, posts }]) => (
          <div key={monthKey} className="month-section">
            <div className="month-separator">
              <h2 className="month-title">{monthName}</h2>
              <div className="month-line"></div>
            </div>
            <div className="page-content-grid">
              {posts.map((post) => (
                <div key={post.slug} className="page-content-item">
                  <PostCard post={post} />
                </div>
              ))}
            </div>
          </div>
        ))}
      </div>
      <script
        // deno-lint-ignore react-no-danger
        dangerouslySetInnerHTML={{
          __html: `
      function updateFadeEffect() {
        if (window.location.pathname !== '/') return;

        const pageContent = document.querySelector('.page-content');
        const scrollTop = window.pageYOffset || document.documentElement.scrollTop;
        const windowHeight = window.innerHeight;
        const documentHeight = document.documentElement.scrollHeight;

        const distanceFromBottom = documentHeight - (scrollTop + windowHeight);

        if (distanceFromBottom < 50) {
          pageContent.classList.add('fade-hidden');
        } else {
          pageContent.classList.remove('fade-hidden');
        }
      }

      window.addEventListener('scroll', updateFadeEffect);
      window.addEventListener('resize', updateFadeEffect);
      document.addEventListener('DOMContentLoaded', updateFadeEffect);

      function markLastRowItems() {
        const monthSections = document.querySelectorAll('.month-section');

        monthSections.forEach(section => {
          const items = Array.from(section.querySelectorAll('.page-content-item'));
          const total = items.length;
          const perRow = 1;
          const lastRowStart = total - (total % perRow || perRow);

          items.forEach((item, index) => {
            item.classList.toggle('last-row-item', index >= lastRowStart);
          });
        });
      }

      const debounce = (fn, wait = 120) => {
        let t;
        return (...args) => { clearTimeout(t); t = setTimeout(() => fn(...args), wait); };
      };

      window.addEventListener('load', markLastRowItems);
      window.addEventListener('resize', debounce(markLastRowItems));
    `,
        }}
      />
    </main>
  );
});
