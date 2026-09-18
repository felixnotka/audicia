# ---- Build stage ----
FROM --platform=$BUILDPLATFORM golang:1.27-alpine@sha256:4cb7ac979db5fcc41cae44b2227ba5ab8a51e8807f40d9ba4dee20a0ad960b5b AS builder

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
FROM alpine:3.24@sha256:5b02b42e375f7426f8d65c3af331ca05d9878f9989230354504e0b9dfd431f60

# Upgrade and create dedicated non-root user in a single layer.
RUN apk --no-cache upgrade && \
    adduser -u 10000 -D -g '' audicia audicia

COPY --from=builder /audicia /usr/local/bin/audicia

USER 10000

ENTRYPOINT ["audicia"]
