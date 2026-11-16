# ---- Build stage ----
FROM --platform=$BUILDPLATFORM golang:1.27-alpine@sha256:70bd7ddea8b6cbe5d6943cde279d6b67feb10cc924fb9100d1d8757c1575b280 AS builder

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
