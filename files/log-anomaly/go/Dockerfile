FROM golang:1.21-alpine AS builder

# Install build dependencies
RUN apk add --no-cache gcc musl-dev sqlite-dev

# Set working directory
WORKDIR /app

# Copy source code first
COPY *.go go.mod ./

# Download dependencies (skip checksum verification for development)
ENV GOSUMDB=off
RUN go mod tidy && go mod download

# Build the binary
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -o log-anomaly-detector .

# Final stage
FROM alpine:3.19

# Install runtime dependencies
RUN apk add --no-cache ca-certificates sqlite tzdata curl

# Create non-root user
RUN addgroup -g 1001 media && \
    adduser -u 1001 -G media -s /bin/sh -D media

# Create app directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/log-anomaly-detector .

# Create directories and set permissions
RUN mkdir -p /app/data /app/patterns && \
    chown -R media:media /app

# Switch to non-root user
USER media

# Expose port
EXPOSE 8085

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=60s --retries=3 \
    CMD curl -f http://localhost:8085/health || exit 1

# Run the binary
CMD ["./log-anomaly-detector"]
