# ---- Build stage ----
FROM --platform=$BUILDPLATFORM golang:1.26-alpine@sha256:c2a1f7b2095d046ae14b286b18413a05bb82c9bca9b25fe7ff5efef0f0826166 AS builder

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
FROM alpine:3.23@sha256:25109184c71bdad752c8312a8623239686a9a2071e8825f20acb8f2198c3f659

# Upgrade and create dedicated non-root user in a single layer.
RUN apk --no-cache upgrade && \
    adduser -u 10000 -D -g '' audicia audicia

COPY --from=builder /audicia /usr/local/bin/audicia

USER 10000

ENTRYPOINT ["audicia"]
