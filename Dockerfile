# Build backend (simple API server, no go-ios dependency)
FROM golang:1.22-alpine AS backend-builder
WORKDIR /app

# Copy go module files first for better caching
COPY backend/go.mod ./

# Copy source code
COPY backend/*.go .

# Ensure go.mod is valid and build
RUN go mod tidy
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /backend .

# Build host binaries (usbmuxd relay)
FROM golang:1.22-alpine AS host-builder
WORKDIR /app

# Copy go module files first for better caching
COPY host/go.mod ./
# go.sum may not exist if there are no dependencies
COPY host/go.su[m] ./

# Copy source code
COPY host/*.go .

# Ensure go.mod is valid
RUN go mod tidy

# Build for macOS (arm64 and amd64)
RUN GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o /host/darwin/mobile-relay .
RUN GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o /host/darwin-amd64/mobile-relay .

# Build for Linux
RUN GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o /host/linux/mobile-relay .
RUN GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o /host/linux-arm64/mobile-relay .

# Final image
FROM alpine:3.19

# Docker Desktop Extension labels (required)
LABEL org.opencontainers.image.title="mobile-relay" \
      org.opencontainers.image.description="Access iOS devices from Docker containers via usbmuxd" \
      org.opencontainers.image.vendor="aluedeke" \
      com.docker.desktop.extension.api.version="0.3.4" \
      com.docker.desktop.extension.icon="icon.png" \
      com.docker.extension.screenshots="" \
      com.docker.extension.detailed-description="Bridges the macOS usbmuxd socket to Docker containers, allowing tools like go-ios and libimobiledevice to access USB-connected iOS devices." \
      com.docker.extension.publisher-url="https://github.com/aluedeke" \
      com.docker.extension.additional-urls="" \
      com.docker.extension.changelog=""

# Install ca-certificates for TLS
RUN apk add --no-cache ca-certificates

# Copy backend binary
COPY --from=backend-builder /backend /backend

# Copy host binaries (Docker Desktop will extract these)
# The relay binary now includes iOS 17.4+ tunnel management via embedded go-ios
COPY --from=host-builder /host /host

# Copy UI
COPY ui /ui

# Copy scripts for auto-start installation
COPY scripts /scripts

# Copy metadata
COPY metadata.json /metadata.json
COPY icon.png /icon.png
COPY docker-compose.yaml /docker-compose.yaml

CMD ["/backend"]
