# Mobile Device Relay - Docker Extension

[![Build](https://github.com/aluedeke/mobile-docker-extension/actions/workflows/build.yaml/badge.svg)](https://github.com/aluedeke/mobile-docker-extension/actions/workflows/build.yaml)
[![Docker Hub](https://img.shields.io/badge/Docker%20Hub-aluedeke%2Fmobile--docker--extension-blue?logo=docker)](https://hub.docker.com/r/aluedeke/mobile-docker-extension)

A Docker Desktop extension that exposes USB-connected iOS devices to Docker containers via the usbmuxd protocol.

> **Note:** This extension works on macOS and Windows. Linux is not supported (Docker runs natively on Linux, so you can access usbmuxd directly).

<p align="center">
  <img src="https://raw.githubusercontent.com/aluedeke/mobile-docker-extension/main/icon.png" alt="Mobile Device Relay" width="256">
</p>

## What it does

This extension creates a bridge between the host's usbmuxd daemon and Docker containers, allowing tools like `go-ios`, `pymobiledevice3`, `libimobiledevice`, and other iOS automation tools to communicate with physical iOS devices from within containers.

```text
┌─────────────────────────────────────────────────────────────────┐
│ macOS Host                                                      │
│                                                                 │
│  iPhone ──USB──► /var/run/usbmuxd                              │
│                        │                                        │
│              mobile-relay (TCP :27015)                         │
│              + tunnel manager (:60105)                         │
│                        │                                        │
└────────────────────────┼────────────────────────────────────────┘
                         │ host.docker.internal:27015
┌────────────────────────┼────────────────────────────────────────┐
│ Docker Desktop VM      ▼                                        │
│              ┌─────────────────┐                                │
│              │ usbmuxd-backend │                                │
│              └────────┬────────┘                                │
│                       │                                         │
│                       ▼                                         │
│     /run/guest-services/.../usbmuxd.sock                       │
│                       │                                         │
│  ┌────────────────────┼────────────────────────────────────────┐│
│  │ Your Container     │                                        ││
│  │                    ▼                                        ││
│  │          /var/run/usbmuxd (volume mount)                    ││
│  │                                                             ││
│  │   go-ios, pymobiledevice3, libimobiledevice, etc.          ││
│  └─────────────────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────────────────┘
```

## Requirements

- Docker Desktop 4.8.0 or later
- iOS device connected via USB
- **macOS**: Xcode or usbmuxd installed (comes with Xcode Command Line Tools)
- **Windows**: [Apple Devices](https://apps.microsoft.com/detail/9np83lwlpz9k) app from Microsoft Store (provides usbmuxd/iTunes drivers)

## Installation

### From source

```bash
# Clone the repository
git clone https://github.com/aluedeke/mobile-docker-extension.git
cd mobile-docker-extension

# Build and install
make install
```

### From Docker Hub (when published)

```bash
docker extension install aluedeke/mobile-docker-extension
```

## Usage

### 1. Start the host relay

The host relay must be running to bridge connections. You can start it from:

#### Option A: Docker Desktop UI

- Open Docker Desktop
- Go to the "USB Muxd" extension tab
- Click "Start Host Relay"

#### Option B: Command line

```bash
# If installed via extension
~/.docker/extensions/aluedeke_mobile-docker-extension/host/darwin/mobile-relay

# Or build and run locally
make run-host
```

### 2. Configure your containers

Mount the usbmuxd socket into your container at `/var/run/usbmuxd`:

#### Docker Compose

```yaml
services:
  ios-automation:
    image: your-image
    volumes:
      - /run/guest-services/aluedeke_mobile-docker-extension/usbmuxd.sock:/var/run/usbmuxd
```

#### Docker run

```bash
docker run -v /run/guest-services/aluedeke_mobile-docker-extension/usbmuxd.sock:/var/run/usbmuxd your-image
```

### 3. Use iOS tools normally

Once configured, iOS tools work as if running on the host:

```bash
# List connected devices with go-ios
docker run --rm \
  -v /run/guest-services/aluedeke_mobile-docker-extension/usbmuxd.sock:/var/run/usbmuxd \
  ghcr.io/danielpaulus/go-ios:latest list

# Use pymobiledevice3
docker run --rm \
  -v /run/guest-services/aluedeke_mobile-docker-extension/usbmuxd.sock:/var/run/usbmuxd \
  python:3.12-slim sh -c "pip install pymobiledevice3 && pymobiledevice3 usbmux list"
```

### 4. iOS 17.4+ tunnel features (go-ios only)

For go-ios iOS 17.4+ features that require tunnels, add the tunnel agent environment variables:

```yaml
services:
  go-ios:
    image: ghcr.io/danielpaulus/go-ios:latest
    volumes:
      - /run/guest-services/aluedeke_mobile-docker-extension/usbmuxd.sock:/var/run/usbmuxd
    environment:
      - GO_IOS_AGENT_HOST=host.docker.internal
      - GO_IOS_AGENT_PORT=60105
```

Note: pymobiledevice3 handles tunnels transparently over the usbmuxd socket and doesn't need these environment variables.

## Examples

See the example directories for complete working examples:

- [examples/go-ios](examples/go-ios) - Using go-ios with the extension
- [examples/pymobiledevice3](examples/pymobiledevice3) - Using pymobiledevice3 with the extension

### Quick Device Check

```bash
# Using go-ios
docker run --rm \
  -v /run/guest-services/aluedeke_mobile-docker-extension/usbmuxd.sock:/var/run/usbmuxd \
  ghcr.io/danielpaulus/go-ios:latest list

# Using pymobiledevice3
cd examples/pymobiledevice3
docker compose build
docker compose run --rm pymobiledevice3 usbmux list
```

## Troubleshooting

### "Connection refused" or socket errors

1. Verify the host relay is running: `pgrep -f mobile-relay`
2. Check the relay is listening: `lsof -i :27015`
3. Ensure the socket file exists: `ls -la /run/guest-services/aluedeke_mobile-docker-extension/`

### "Connection refused" on host relay

1. Check if usbmuxd is running on the host: `ls -la /var/run/usbmuxd`
2. On macOS, connect an iOS device or start Xcode
3. On Linux, ensure usbmuxd service is running: `systemctl status usbmuxd`

### No devices showing up

1. Verify the device is connected and trusted
2. Check host usbmuxd directly: `idevice_id -l` (on host)
3. View relay logs for connection issues

### iOS 17.4+ tunnel features not working (go-ios)

1. Ensure `GO_IOS_AGENT_HOST` and `GO_IOS_AGENT_PORT` environment variables are set
2. Check the host relay tunnel manager is running (port 60105)
3. View host relay logs for tunnel errors

## Development

### Build locally

```bash
# Build everything
make build

# Build just the host binary
make build-host

# Build just the backend
make build-backend
```

### Run tests

```bash
# Run the full test suite
./test.sh

# Test socket connectivity
make test-socket
```

### Debug mode

```bash
# Enable extension debug mode
make debug

# View logs
make logs

# Reset debug mode
make reset
```

## How it works

1. **Host Relay** (`mobile-relay`): A Go binary that runs on the host, listens on TCP port 27015, and forwards connections to the real `/var/run/usbmuxd` socket. Also includes a tunnel manager on port 60105 for iOS 17.4+ devices (go-ios compatible).

2. **Backend Service** (`usbmuxd-backend`): Runs inside the Docker Desktop VM. Creates a Unix socket that proxies connections to the host relay. Provides an API for the extension UI.

3. **Container Configuration**: Containers mount the backend's Unix socket at `/var/run/usbmuxd`. iOS tools use this standard location automatically.

## License

MIT
