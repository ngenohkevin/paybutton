# Multi-stage build for smaller image size
FROM golang:1.23-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates

WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build with optimizations for smaller binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s" \
    -trimpath \
    -o paybutton \
    main.go

# Final stage - mid
FROM alpine:latest

# Install runtime dependencies
RUN apk --no-cache add ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1000 -S paybutton && \
    adduser -u 1000 -S paybutton -G paybutton

WORKDIR /app

# Copy binary and necessary files from builder
COPY --from=builder /app/paybutton .
COPY --from=builder /app/templates ./templates
COPY --from=builder /app/static ./static

# Change ownership
RUN chown -R paybutton:paybutton /app

# Switch to non-root user
USER paybutton

# Expose port
EXPOSE 8000

# Health check for Render
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8000/ || exit 1

# Set resource-conscious environment variables
ENV GOMAXPROCS=2 \
    GOGC=20 \
    PORT=8000 \
    MAX_MEMORY_MB=400 \
    MAX_GOROUTINES=50 \
    MAX_IDLE_CONNS=5 \
    MAX_OPEN_CONNS=10 \
    COOLIFY_DEPLOYMENT=true

# Run the binary
CMD ["./paybutton"]