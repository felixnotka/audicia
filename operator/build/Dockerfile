# ---- Build stage ----
FROM --platform=$BUILDPLATFORM golang:1.26-alpine@sha256:f1ddd9fe14fffc091dd98cb4bfa999f32c5fc77d2f2305ea9f0e2595c5437c14 AS builder

RUN apk --no-cache add git

WORKDIR /workspace

# Cache dependencies.
COPY go.mod go.sum ./
RUN go mod download

# Copy only Go source directories needed for compilation.
COPY cmd/ cmd/
COPY pkg/ pkg/

# Build with static linking for alpine compatibility.
# Set GO_BUILD_TAGS to include cloud provider adapters (e.g., "azure").
ARG VERSION=dev
ARG COMMIT=none
ARG DATE=unknown
ARG GO_BUILD_TAGS=""
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build \
    -tags "${GO_BUILD_TAGS}" \
    -ldflags "-X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE}" \
    -o /audicia \
    ./cmd/audicia/

# ---- Runtime stage ----
FROM alpine:3.24@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b

# Upgrade and create dedicated non-root user in a single layer.
RUN apk --no-cache upgrade && \
    adduser -u 10000 -D -g '' audicia audicia

COPY --from=builder /audicia /usr/local/bin/audicia

USER 10000

ENTRYPOINT ["audicia"]
