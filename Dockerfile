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

# Build for Windows
RUN GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o /host/windows/mobile-relay.exe .

# Final image
FROM alpine:3.19

# Docker Desktop Extension labels (required)
LABEL org.opencontainers.image.title="Mobile Device Relay" \
      org.opencontainers.image.description="Access USB-connected iOS devices from Docker containers (macOS only)" \
      org.opencontainers.image.vendor="aluedeke" \
      org.opencontainers.image.source="https://github.com/aluedeke/usbmuxd-docker-extension" \
      org.opencontainers.image.licenses="MIT" \
      com.docker.desktop.extension.api.version="0.3.4" \
      com.docker.desktop.extension.icon="icon.png" \
      com.docker.extension.categories="utility" \
      com.docker.extension.detailed-description="Bridges the macOS usbmuxd socket to Docker containers, enabling tools like go-ios, pymobiledevice3, and libimobiledevice to communicate with USB-connected iOS devices. Includes iOS 17.4+ tunnel support. NOTE: Currently only tested on macOS." \
      com.docker.extension.publisher-url="https://github.com/aluedeke" \
      com.docker.extension.additional-urls="[{\"title\":\"Documentation\",\"url\":\"https://github.com/aluedeke/usbmuxd-docker-extension#readme\"},{\"title\":\"Issues\",\"url\":\"https://github.com/aluedeke/usbmuxd-docker-extension/issues\"}]" \
      com.docker.extension.changelog="Initial release with iOS 17.4+ tunnel support"

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
