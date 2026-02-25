import { DOCS_NAV } from "../lib/docs.ts";
import type { NavSection } from "../lib/docs.ts";

interface Props {
  currentPath: string;
}

export default function DocsSidebar({ currentPath }: Readonly<Props>) {
  return (
    <aside className="docs-sidebar">
      <button
        type="button"
        className="docs-sidebar-toggle"
        aria-label="Toggle documentation menu"
      >
        Menu <span className="docs-sidebar-toggle-icon">+</span>
      </button>
      <nav className="docs-sidebar-nav" aria-label="Documentation">
        <div className="docs-search-container">
          <div className="docs-search-input-wrapper">
            <input
              type="search"
              className="docs-search-input"
              placeholder="Search docs..."
              aria-label="Search documentation"
              autoComplete="off"
            />
            <kbd className="docs-search-shortcut">
              <span className="docs-search-shortcut-key" />K
            </kbd>
          </div>
          <div
            className="docs-search-results"
            role="listbox"
            aria-label="Search results"
          />
        </div>
        <label className="docs-expert-toggle">
          <input
            type="checkbox"
            className="docs-expert-toggle-input"
            aria-label="Toggle expert mode"
          />
          <span className="docs-expert-toggle-track">
            <span className="docs-expert-toggle-thumb" />
          </span>
          <span className="docs-expert-toggle-label">Expert Mode</span>
        </label>
        {DOCS_NAV.map((section: NavSection) => (
          <div
            className={`docs-sidebar-section${
              section.expert ? " docs-expert-section" : ""
            }`}
            key={section.slug || "_root"}
          >
            {section.section && (
              <div className="docs-sidebar-section-title">
                {section.expert ? `+ ${section.section}` : section.section}
              </div>
            )}
            <ul className="docs-sidebar-list">
              {section.pages.map((page) => {
                const href = section.slug
                  ? `/docs/${section.slug}/${page.slug}`
                  : `/docs/${page.slug}`;
                const isActive = currentPath === href;
                return (
                  <li key={page.slug}>
                    <a
                      href={href}
                      className={`docs-sidebar-link${
                        isActive ? " active" : ""
                      }`}
                      aria-current={isActive ? "page" : undefined}
                    >
                      {page.title}
                    </a>
                  </li>
                );
              })}
            </ul>
          </div>
        ))}
      </nav>
    </aside>
  );
}
