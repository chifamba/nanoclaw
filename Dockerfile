# Stage 1: Build the Go binary
FROM golang:1.26-alpine AS builder

# Install build dependencies for CGO (required by mattn/go-sqlite3)
RUN apk add --no-cache gcc musl-dev

WORKDIR /app

# Leverage Docker cache for dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the nanoclaw binary for Linux
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -o nanoclaw ./cmd/nanoclaw

# Stage 2: Final production image
FROM alpine:latest

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

# Copy the binary from the builder stage
COPY --from=builder /app/nanoclaw .

# Ensure standard directory structure exists
RUN mkdir -p groups data store logs

# Default environment variables
ENV ASSISTANT_NAME=Andy
ENV CREDENTIAL_PROXY_PORT=3001
ENV TZ=UTC

# Run the host orchestrator
ENTRYPOINT ["./nanoclaw"]
