# ============================================================
# Control Panel API Dockerfile
# Multi-stage build: compile Go binary, then run on minimal image
# ============================================================

# --- Build Stage ---
FROM golang:1.22-alpine AS builder

RUN apk add --no-cache git

WORKDIR /app

# Copy dependency files first (Docker caches this layer)
COPY go.mod go.sum ./
RUN go mod download

# Copy source code and build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /control-panel ./main.go

# --- Runtime Stage ---
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

# Copy the compiled binary from the build stage
COPY --from=builder /control-panel .

# Run as non-root user
RUN adduser -D -u 1000 appuser
USER appuser

EXPOSE 8080

# Go binaries are already production-ready — no extra server needed
CMD ["./control-panel"]
