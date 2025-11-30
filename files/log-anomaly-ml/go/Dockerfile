FROM golang:1.22-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache gcc musl-dev

# Copy all source files first
COPY go.mod *.go ./

# Download dependencies and generate go.sum
RUN go mod tidy && go mod download

# Build
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o log-anomaly-ml-processor .

# Runtime stage
FROM alpine:3.19

WORKDIR /app

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata curl

# Create non-root user
RUN addgroup -g 1001 -S appgroup && \
    adduser -u 1001 -S appuser -G appgroup

# Copy binary
COPY --from=builder /app/log-anomaly-ml-processor .

# Set ownership
RUN chown -R appuser:appgroup /app

USER appuser

EXPOSE 8087

CMD ["./log-anomaly-ml-processor"]
