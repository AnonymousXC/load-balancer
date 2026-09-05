# Deployment Guide

This guide covers various deployment scenarios for the L7 load balancer.

## Prerequisites

- Go 1.25.4+ (for building from source)
- Linux/macOS/Windows (for deployment)
- Basic networking knowledge
- TLS certificates (for production deployment)

## Build from Source

### Development Build
```bash
# Clone repository
git clone https://github.com/yourusername/load-balancer.git
cd load-balancer

# Install dependencies
go mod download

# Build
go build -o l7-proxy ./cmd/proxy

# Run
./l7-proxy
```

### Production Build
```bash
# Build with optimizations
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
  -ldflags="-s -w" \
  -o l7-proxy \
  ./cmd/proxy

# Verify build
./l7-proxy --version
```
