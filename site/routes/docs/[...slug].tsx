import { Head } from "fresh/runtime";
import { HttpError } from "fresh";
import { define } from "../../utils.ts";
import { buildSearchIndex, getDoc, renderDoc } from "../../lib/docs.ts";
import { CSS } from "@deno/gfm";
import DocsSidebar from "../../components/DocsSidebar.tsx";

export const handler = define.handlers({
  async GET(ctx) {
    const slugParts = ctx.params.slug.split("/");

    // Block internal docs
    if (slugParts[0] === "internal") {
      throw new HttpError(404, "Page not found");
    }

    const doc = await getDoc(...slugParts);

    if (!doc) {
      throw new HttpError(404, "Page not found");
    }

    ctx.state.title = `${doc.title} — Audicia Docs`;
    ctx.state.description = `Audicia documentation: ${doc.title}`;

    const searchIndex = await buildSearchIndex();

    return { data: { doc, searchIndex } };
  },
});

export default define.page<typeof handler>(function DocsPage(props) {
  const { doc, searchIndex } = props.data;

  return (
    <div>
      <Head>
        <meta property="og:type" content="article" />
        <meta property="og:title" content={`${doc.title} — Audicia Docs`} />
      </Head>
      <style dangerouslySetInnerHTML={{ __html: CSS }} />
      <script
        type="application/json"
        id="docs-search-index"
        dangerouslySetInnerHTML={{ __html: JSON.stringify(searchIndex) }}
      />
      <script
        // deno-lint-ignore react-no-danger
        dangerouslySetInnerHTML={{
          __html: String.raw`
          document.addEventListener('DOMContentLoaded', function() {
            var sidebar = document.querySelector('.docs-sidebar');

            // Mobile menu toggle
            var toggle = document.querySelector('.docs-sidebar-toggle');
            var nav = document.querySelector('.docs-sidebar-nav');
            if (toggle && nav) {
              toggle.addEventListener('click', function() {
                var open = nav.classList.toggle('open');
                toggle.querySelector('.docs-sidebar-toggle-icon').textContent = open ? '\u2212' : '+';
              });
            }

            // Expert mode toggle
            var expertCheckbox = document.querySelector('.docs-expert-toggle-input');
            if (expertCheckbox && sidebar) {
              var activeInExpert = sidebar.querySelector('.docs-expert-section .docs-sidebar-link.active');
              var stored = localStorage.getItem('docs-expert-mode');
              if (stored === 'true' || activeInExpert) {
                expertCheckbox.checked = true;
                sidebar.classList.add('expert-mode');
                if (activeInExpert) localStorage.setItem('docs-expert-mode', 'true');
              }
              expertCheckbox.addEventListener('change', function() {
                sidebar.classList.toggle('expert-mode', expertCheckbox.checked);
                localStorage.setItem('docs-expert-mode', String(expertCheckbox.checked));
              });
            }

            // Restore sidebar scroll position after navigation
            if (sidebar) {
              var savedScroll = sessionStorage.getItem('docs-sidebar-scroll');
              if (savedScroll) {
                sidebar.scrollTop = parseInt(savedScroll, 10);
                sessionStorage.removeItem('docs-sidebar-scroll');
              }
              // Save scroll position before navigating away
              sidebar.querySelectorAll('a').forEach(function(link) {
                link.addEventListener('click', function() {
                  sessionStorage.setItem('docs-sidebar-scroll', String(sidebar.scrollTop));
                });
              });
            }

            // ── Search ──
            var searchEl = document.getElementById('docs-search-index');
            var searchInput = document.querySelector('.docs-search-input');
            var searchResults = document.querySelector('.docs-search-results');
            var shortcutKey = document.querySelector('.docs-search-shortcut-key');

            if (searchEl && searchInput && searchResults) {
              var searchIndex = JSON.parse(searchEl.textContent);
              var selectedIdx = -1;

              // Platform-appropriate shortcut hint
              if (shortcutKey) {
                shortcutKey.textContent = navigator.platform.indexOf('Mac') > -1 ? '\u2318' : 'Ctrl+';
              }

              function doSearch(query) {
                searchResults.innerHTML = '';
                selectedIdx = -1;
                if (!query || query.length < 2) {
                  searchResults.classList.remove('visible');
                  return;
                }
                var tokens = query.toLowerCase().split(/\s+/).filter(Boolean);
                var scored = [];

                for (var i = 0; i < searchIndex.length; i++) {
                  var entry = searchIndex[i];
                  var score = 0;
                  var matchedHeading = null;
                  var allMatched = true;

                  for (var j = 0; j < tokens.length; j++) {
                    var tok = tokens[j];
                    var tokMatched = false;
                    if (entry.t.toLowerCase().indexOf(tok) > -1) { score += 10; tokMatched = true; }
                    if (entry.s.toLowerCase().indexOf(tok) > -1) { score += 3; tokMatched = true; }
                    if (entry.x.toLowerCase().indexOf(tok) > -1) { score += 1; tokMatched = true; }
                    for (var k = 0; k < entry.h.length; k++) {
                      if (entry.h[k][0].toLowerCase().indexOf(tok) > -1) {
                        score += 5; tokMatched = true;
                        if (!matchedHeading) matchedHeading = entry.h[k];
                      }
                    }
                    if (!tokMatched) { allMatched = false; break; }
                  }
                  if (allMatched && score > 0) {
                    scored.push({ entry: entry, score: score, heading: matchedHeading });
                  }
                }

                scored.sort(function(a, b) {
                  return b.score - a.score || a.entry.t.localeCompare(b.entry.t);
                });
                var results = scored.slice(0, 8);

                if (results.length === 0) {
                  searchResults.innerHTML = '<div class="docs-search-empty">No results found</div>';
                  searchResults.classList.add('visible');
                  return;
                }

                for (var r = 0; r < results.length; r++) {
                  var item = results[r];
                  var href = item.entry.p;
                  if (item.heading) href += '#' + item.heading[1];
                  var div = document.createElement('a');
                  div.href = href;
                  div.className = 'docs-search-result-item';
                  div.setAttribute('role', 'option');
                  div.innerHTML =
                    '<span class="docs-search-result-section">' + item.entry.s + '</span>' +
                    '<span class="docs-search-result-title">' + item.entry.t + '</span>' +
                    (item.heading
                      ? '<span class="docs-search-result-heading"># ' + item.heading[0] + '</span>'
                      : '');
                  searchResults.appendChild(div);
                }
                searchResults.classList.add('visible');
              }

              // Debounced input
              var searchTimer = null;
              searchInput.addEventListener('input', function() {
                clearTimeout(searchTimer);
                searchTimer = setTimeout(function() {
                  doSearch(searchInput.value.trim());
                }, 150);
              });

              // Keyboard navigation in results
              searchInput.addEventListener('keydown', function(e) {
                var items = searchResults.querySelectorAll('.docs-search-result-item');
                if (e.key === 'ArrowDown') {
                  e.preventDefault();
                  if (items.length) { selectedIdx = Math.min(selectedIdx + 1, items.length - 1); updateSel(items); }
                } else if (e.key === 'ArrowUp') {
                  e.preventDefault();
                  if (items.length) { selectedIdx = Math.max(selectedIdx - 1, -1); updateSel(items); }
                } else if (e.key === 'Enter' && selectedIdx >= 0 && items[selectedIdx]) {
                  e.preventDefault();
                  items[selectedIdx].click();
                } else if (e.key === 'Escape') {
                  searchInput.value = '';
                  searchResults.classList.remove('visible');
                  searchInput.blur();
                }
              });

              function updateSel(items) {
                for (var i = 0; i < items.length; i++) {
                  items[i].classList.toggle('selected', i === selectedIdx);
                }
                if (selectedIdx >= 0 && items[selectedIdx]) items[selectedIdx].scrollIntoView({ block: 'nearest' });
              }

              // Close on outside click
              document.addEventListener('click', function(e) {
                if (!e.target.closest('.docs-search-container')) {
                  searchResults.classList.remove('visible');
                }
              });

              // Cmd+K / Ctrl+K global shortcut
              document.addEventListener('keydown', function(e) {
                if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
                  e.preventDefault();
                  searchInput.focus();
                  searchInput.select();
                }
              });
            }
          });
        `,
        }}
      />
      <div className="docs-layout">
        <DocsSidebar currentPath={doc.path} />
        <main id="main" className="docs-content">
          <div
            className="docs-body markdown-body"
            // deno-lint-ignore react-no-danger
            dangerouslySetInnerHTML={{ __html: renderDoc(doc.content) }}
          />
        </main>
      </div>
    </div>
  );
});
