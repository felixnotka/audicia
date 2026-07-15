# ============================================================================
# Stage 1: Build
# ============================================================================
# Build context is the repo root (docker build -f site/Dockerfile .)
FROM denoland/deno:2.9.3@sha256:6288db6be1c26473bb9d1843f906d9a219c0dda57fd85de4aac90296951cf6ef AS build
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
FROM denoland/deno:2.9.3@sha256:6288db6be1c26473bb9d1843f906d9a219c0dda57fd85de4aac90296951cf6ef
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
