import { join } from "@std/path";
import { render } from "@deno/gfm";
import GithubSlugger from "github-slugger";

const DOCS_DIR = "../docs";

export interface DocPage {
  slug: string;
  category: string | null;
  title: string;
  content: string;
  path: string; // URL path e.g. "/docs/concepts/architecture"
}

export interface NavSection {
  section: string | null;
  slug: string | null;
  pages: { slug: string; title: string }[];
  expert?: boolean;
}

export const DOCS_NAV: NavSection[] = [
  {
    section: "Getting Started",
    slug: "getting-started",
    pages: [
      { slug: "introduction", title: "Introduction" },
      { slug: "installation", title: "Installation" },
      { slug: "quick-start-file", title: "Quick Start: File Mode" },
      { slug: "quick-start-webhook", title: "Quick Start: Webhook Mode" },
      { slug: "quick-start-aks", title: "Quick Start: AKS Cloud Ingestion" },
      { slug: "quick-start-eks", title: "Quick Start: EKS Cloud Ingestion" },
      { slug: "quick-start-gke", title: "Quick Start: GKE Cloud Ingestion" },
    ],
  },
  {
    section: "Concepts",
    slug: "concepts",
    pages: [
      { slug: "architecture", title: "Architecture" },
      { slug: "pipeline", title: "Pipeline" },
      { slug: "compliance-scoring", title: "Compliance Scoring" },
      { slug: "rbac-generation", title: "RBAC Policy Generation" },
      { slug: "security-model", title: "Security Model" },
      { slug: "cloud-ingestion", title: "Cloud Ingestion" },
    ],
  },
  {
    section: "Setup Guides",
    slug: "guides",
    pages: [
      { slug: "audit-policy", title: "Audit Policy" },
      { slug: "webhook-setup", title: "Webhook Setup" },
      { slug: "kube-proxy-free", title: "Kube-Proxy-Free Clusters" },
      { slug: "aks-setup", title: "AKS Setup (Event Hub)" },
      { slug: "eks-setup", title: "EKS Setup (CloudWatch Logs)" },
      { slug: "gke-setup", title: "GKE Setup (Pub/Sub)" },
    ],
  },
  {
    section: "Guides",
    slug: "guides",
    pages: [
      { slug: "filter-recipes", title: "Filter Recipes" },
      { slug: "demo-walkthrough", title: "Demo Walkthrough" },
    ],
  },
  {
    section: "Components",
    slug: "components",
    expert: true,
    pages: [
      { slug: "ingestor", title: "Ingestor" },
      { slug: "filter", title: "Filter" },
      { slug: "normalizer", title: "Normalizer" },
      { slug: "aggregator", title: "Aggregator" },
      { slug: "strategy-engine", title: "Strategy Engine" },
      { slug: "compliance-engine", title: "Compliance Engine" },
      { slug: "controller", title: "Controller" },
    ],
  },
  {
    section: "Configuration",
    slug: "configuration",
    expert: true,
    pages: [
      { slug: "helm-values", title: "Helm Values" },
    ],
  },
  {
    section: "API Reference",
    slug: "reference",
    expert: true,
    pages: [
      { slug: "crd-audiciasource", title: "AudiciaSource CRD" },
      { slug: "crd-audiciapolicyreport", title: "AudiciaPolicyReport CRD" },
      { slug: "features", title: "Features" },
      { slug: "metrics", title: "Metrics" },
    ],
  },
  {
    section: "Examples",
    slug: "examples",
    expert: true,
    pages: [
      { slug: "audit-policy", title: "Audit Policy" },
      { slug: "audicia-source-file", title: "AudiciaSource: File" },
      { slug: "audicia-source-webhook", title: "AudiciaSource: Webhook" },
      { slug: "audicia-source-hardened", title: "AudiciaSource: Hardened" },
      { slug: "audicia-source-cloud-aks", title: "AudiciaSource: Cloud (AKS)" },
      { slug: "audicia-source-cloud-eks", title: "AudiciaSource: Cloud (EKS)" },
      { slug: "audicia-source-cloud-gke", title: "AudiciaSource: Cloud (GKE)" },
      { slug: "webhook-kubeconfig", title: "Webhook Kubeconfig" },
      { slug: "webhook-kubeconfig-mtls", title: "Webhook Kubeconfig (mTLS)" },
      { slug: "network-policy", title: "NetworkPolicy" },
      { slug: "policy-report", title: "Policy Report" },
    ],
  },
  {
    section: "Project",
    slug: null,
    pages: [
      { slug: "troubleshooting", title: "Troubleshooting" },
      { slug: "limitations", title: "Limitations" },
      { slug: "comparisons", title: "Comparisons" },
      { slug: "roadmap", title: "Roadmap" },
      { slug: "changelog", title: "Changelog" },
    ],
  },
];

/** Flat ordered list of all pages for prev/next navigation */
export interface FlatPage {
  slug: string;
  category: string | null;
  title: string;
  path: string;
}

export function getFlatPages(): FlatPage[] {
  const pages: FlatPage[] = [];
  for (const section of DOCS_NAV) {
    for (const page of section.pages) {
      const path = section.slug
        ? `/docs/${section.slug}/${page.slug}`
        : `/docs/${page.slug}`;
      pages.push({
        slug: page.slug,
        category: section.slug,
        title: page.title,
        path,
      });
    }
  }
  return pages;
}

/** Transform relative markdown links to site-internal doc links */
function transformLinks(markdown: string, category: string | null): string {
  // Regex note: the path segment uses [a-zA-Z0-9_/-]+ with dots handled
  // explicitly via (?:\.[a-zA-Z0-9_/-]+)* to prevent backtracking overlap
  // between the character class and the literal "\.md" that follows.
  return markdown.replaceAll(
    /\[([^\]]+)\]\(((?:\.\.\/|\.\/)?[a-zA-Z0-9_/-]+(?:\.[a-zA-Z0-9_/-]+)*)\.md(#[a-zA-Z0-9_-]*)?\)/g,
    (_match: string, text: string, basePath: string, anchor = "") => {
      const fragment = anchor;

      // Handle cross-category links like ../guides/audit-policy
      if (basePath.startsWith("../")) {
        const resolved = basePath.replaceAll("../", "");
        return `[${text}](/docs/${resolved}${fragment})`;
      }

      // Handle ./file or file (same category)
      const slug = basePath.replaceAll("./", "");
      if (category) {
        return `[${text}](/docs/${category}/${slug}${fragment})`;
      }
      return `[${text}](/docs/${slug}${fragment})`;
    },
  );
}

export async function getDoc(
  ...slugParts: string[]
): Promise<DocPage | null> {
  try {
    let filePath: string;
    let category: string | null;

    if (slugParts.length === 2) {
      // e.g. ["concepts", "architecture"]
      category = slugParts[0];
      if (category === "internal") return null;
      filePath = join(DOCS_DIR, category, `${slugParts[1]}.md`);
    } else if (slugParts.length === 1) {
      // e.g. ["roadmap"]
      category = null;
      filePath = join(DOCS_DIR, `${slugParts[0]}.md`);
    } else {
      return null;
    }

    const text = await Deno.readTextFile(filePath);

    // Extract title from first # heading
    const titleMatch = /^#\s+([^\n]+)$/m.exec(text);
    const title = titleMatch ? titleMatch[1].trim() : slugParts.at(-1);

    const transformed = transformLinks(text, category);

    const path = category
      ? `/docs/${category}/${slugParts[1]}`
      : `/docs/${slugParts[0]}`;

    return {
      slug: slugParts.at(-1),
      category,
      title,
      content: transformed,
      path,
    };
  } catch (error) {
    console.error(`Failed to load doc "${slugParts.join("/")}":`, error);
    return null;
  }
}

export function renderDoc(content: string): string {
  return render(content);
}

// ── Search Index ──

export interface SearchEntry {
  /** Page title */
  t: string;
  /** URL path */
  p: string;
  /** Section name */
  s: string;
  /** [heading text, anchor id] tuples for h2/h3 */
  h: [string, string][];
  /** Plain-text snippet (~160 chars) */
  x: string;
  /** Expert-only page */
  e: boolean;
}

let _searchIndex: SearchEntry[] | null = null;

async function buildSearchEntryForPage(
  section: NavSection,
  page: { slug: string; title: string },
  slugger: GithubSlugger,
): Promise<SearchEntry | null> {
  const filePath = section.slug
    ? join(DOCS_DIR, section.slug, `${page.slug}.md`)
    : join(DOCS_DIR, `${page.slug}.md`);

  let text: string;
  try {
    text = await Deno.readTextFile(filePath);
  } catch {
    return null;
  }

  const urlPath = section.slug
    ? `/docs/${section.slug}/${page.slug}`
    : `/docs/${page.slug}`;

  // Extract title
  const titleMatch = /^#\s+([^\n]+)$/m.exec(text);
  const title = titleMatch ? titleMatch[1].trim() : page.title;

  // Extract h2/h3 headings with anchor slugs
  slugger.reset();
  const headings: [string, string][] = [];
  const headingRegex = /^#{2,3}\s+([^\n]+)$/gm;
  let match;
  while ((match = headingRegex.exec(text)) !== null) {
    const headingText = match[1].trim();
    const anchor = slugger.slug(headingText);
    headings.push([headingText, anchor]);
  }

  // Generate plain-text snippet
  const snippet = text
    .replaceAll(/^#{1,6}\s+[^\n]+$/gm, "") // remove headings
    .replaceAll(/```[^`]*(?:`(?!``)[^`]*)*```/g, "") // remove code blocks
    .replaceAll(/`[^`]+`/g, "") // remove inline code
    .replaceAll(/\[([^\]]+)\]\([^)]+\)/g, "$1") // links → text
    .replaceAll(/[*_~]/g, "") // remove emphasis
    .replaceAll(/\|[^\n]+\|/g, "") // remove table rows
    .replaceAll(/---+/g, "") // remove hr
    .replaceAll(/\n{2,}/g, " ")
    .replaceAll("\n", " ")
    .trim()
    .slice(0, 160);

  return {
    t: title,
    p: urlPath,
    s: section.section || "Project",
    h: headings,
    x: snippet,
    e: !!section.expert,
  };
}

export async function buildSearchIndex(): Promise<SearchEntry[]> {
  if (_searchIndex) return _searchIndex;

  const entries: SearchEntry[] = [];
  const slugger = new GithubSlugger();

  for (const section of DOCS_NAV) {
    for (const page of section.pages) {
      const entry = await buildSearchEntryForPage(section, page, slugger);
      if (entry) entries.push(entry);
    }
  }

  _searchIndex = entries;
  return entries;
}
