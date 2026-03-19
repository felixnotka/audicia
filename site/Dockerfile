# ============================================================================
# Stage 1: Build
# ============================================================================
# Build context is the repo root (docker build -f site/Dockerfile .)
FROM denoland/deno:2.7.7@sha256:955798026c6ab497f99111345b25cd47857ef366bf70eed3614deba66bd86d1a AS build
WORKDIR /app

# Copy site source
COPY site/ ./site/

# Copy docs from repo root (single source of truth)
COPY docs/ ./docs/

# Build from the site directory
WORKDIR /app/site
RUN deno task build

# ===========================================================================
# Stage 2: Runtime
# ============================================================================
FROM denoland/deno:2.7.7@sha256:955798026c6ab497f99111345b25cd47857ef366bf70eed3614deba66bd86d1a
WORKDIR /app/site

# Built Fresh server bundle
COPY --from=build /app/site/_fresh ./_fresh

# Blog posts (read at runtime)
COPY --from=build /app/site/blog ./blog

# Docs (read at runtime via DOCS_DIR = "../docs")
COPY --from=build /app/docs ../docs

# Helm chart assets (populated by CI before build)
COPY site/static/charts/ ./static/charts/

RUN chown -R deno:deno /app

EXPOSE 8000
USER deno

CMD ["serve", "-A", "_fresh/server.js"]
