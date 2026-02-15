# Multi-architecture Dockerfile
# Build: docker build --platform linux/amd64 -t ai-corp .
# Multi-arch: docker buildx build --platform linux/amd64,linux/arm64 -t ai-corp .

# Stage 1: Build frontend
FROM --platform=$BUILDPLATFORM node:20-alpine AS frontend-builder

WORKDIR /web

# Copy package files first for caching
COPY web/package*.json ./
RUN npm ci --silent

# Copy web source (excluding node_modules via .dockerignore)
COPY web/src ./src
COPY web/index.html web/vite.config.ts web/tsconfig.json web/tsconfig.node.json web/postcss.config.js web/tailwind.config.js ./

# Build frontend using npx to avoid permission issues
RUN npx tsc && npx vite build

# Stage 2: Build Go backend
FROM --platform=$BUILDPLATFORM golang:1.22-alpine AS builder

# Build arguments for cross-compilation
ARG TARGETOS
ARG TARGETARCH

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache gcc musl-dev git

# Copy go.mod first for better caching
COPY go/go.mod ./
RUN go mod download || true

# Copy all source files
COPY go/*.go ./

# Download dependencies and generate go.sum
RUN go mod tidy && go mod download

# Build for target platform
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build -ldflags="-s -w" -o ai-corp .

# Test stage (optional, for CI)
FROM builder AS tester
RUN go test -v -short ./...

# Runtime stage
FROM --platform=$TARGETPLATFORM alpine:3.19 AS runtime

WORKDIR /app

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata curl

# Create non-root user
RUN addgroup -g 1001 -S appgroup && \
    adduser -u 1001 -S appuser -G appgroup

# Create directories
RUN mkdir -p /app/workflows /app/web/dist /app/artifacts && \
    chown -R appuser:appgroup /app

# Copy binary from builder
COPY --from=builder /app/ai-corp .

# Copy frontend build from frontend-builder
COPY --from=frontend-builder /web/dist /app/web/dist

# Set ownership
RUN chown -R appuser:appgroup /app

USER appuser

EXPOSE 8088

HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8088/health || exit 1

CMD ["./ai-corp"]
