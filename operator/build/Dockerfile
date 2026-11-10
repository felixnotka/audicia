# ---- Build stage ----
FROM --platform=$BUILDPLATFORM golang:1.27-alpine@sha256:eb61c3873c97416fff0116553fff65d67a3549f72e11d9758849a689cdd8b523 AS builder

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
FROM alpine:3.24@sha256:f59abe656b692d901e184be3445e084786602108599d2a51d229c0f5a48c71bf

# Upgrade and create dedicated non-root user in a single layer.
RUN apk --no-cache upgrade && \
    adduser -u 10000 -D -g '' audicia audicia

COPY --from=builder /audicia /usr/local/bin/audicia

USER 10000

ENTRYPOINT ["audicia"]
