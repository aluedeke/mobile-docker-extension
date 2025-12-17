# USB Muxd Docker Extension

A Docker Desktop extension that exposes macOS/Linux USB-connected iOS devices to Docker containers via the usbmuxd protocol.

## What it does

This extension creates a bridge between the host's usbmuxd daemon and Docker containers, allowing tools like `go-ios`, `libimobiledevice`, and other iOS automation tools to communicate with physical iOS devices from within containers.

```
┌─────────────────────────────────────────────────────────────────┐
│ macOS Host                                                      │
│                                                                 │
│  iPhone ──USB──► /var/run/usbmuxd                              │
│                        ↑                                        │
│                  usbmuxd-relay (TCP :27015)                     │
│                        ↑                                        │
└────────────────────────┼────────────────────────────────────────┘
                         │ host.docker.internal:27015
┌────────────────────────┼────────────────────────────────────────┐
│ Docker Desktop VM      ↓                                        │
│                  ┌─────────────┐                                │
│                  │   Backend   │                                │
│                  └──────┬──────┘                                │
│                         │                                       │
│              /run/usbmuxd-shared/usbmuxd                        │
│                         │                                       │
│              Volume: usbmuxd-socket                             │
│                         │                                       │
│  ┌──────────────────────┼──────────────────────────────────┐   │
│  │ Your Container       ↓                                   │   │
│  │           /var/run/usbmuxd                               │   │
│  │                                                          │   │
│  │   go-ios, libimobiledevice, idevice_id, etc.            │   │
│  └──────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

## Requirements

- Docker Desktop 4.8.0 or later
- macOS with Xcode/usbmuxd or Linux with usbmuxd installed
- iOS device connected via USB

## Installation

### From source

```bash
# Clone the repository
git clone https://github.com/aluedeke/usbmuxd-docker-extension.git
cd usbmuxd-docker-extension

# Build and install
make install
```

### From Docker Hub (when published)

```bash
docker extension install aluedeke/usbmuxd-docker-extension
```

## Usage

### 1. Start the host relay

The host relay must be running to bridge connections. You can start it from:

**Option A: Docker Desktop UI**
- Open Docker Desktop
- Go to the "USB Muxd" extension tab
- Click "Start Host Relay"

**Option B: Command line**
```bash
# If installed via extension
~/.docker/extensions/aluedeke_usbmuxd-docker-extension/host/darwin/usbmuxd-relay

# Or build and run locally
make run-host
```

### 2. Mount the socket in your containers

Add the `usbmuxd-socket` volume to your container:

**Docker run:**
```bash
docker run -v usbmuxd-socket:/var/run/usbmuxd your-image
```

**Docker Compose:**
```yaml
services:
  ios-automation:
    image: your-image
    volumes:
      - usbmuxd-socket:/var/run/usbmuxd

volumes:
  usbmuxd-socket:
    external: true
```

### 3. Use iOS tools normally

Once the socket is mounted, iOS tools work as if running on the host:

```bash
# List connected devices with go-ios
docker run --rm -v usbmuxd-socket:/var/run/usbmuxd \
  ghcr.io/danielpaulus/go-ios:latest list

# Use libimobiledevice tools
docker run --rm -v usbmuxd-socket:/var/run/usbmuxd \
  alpine sh -c "apk add libimobiledevice && idevice_id -l"
```

## Examples

### iOS Test Automation

```yaml
# docker-compose.yaml
services:
  appium:
    image: appium/appium
    volumes:
      - usbmuxd-socket:/var/run/usbmuxd
    ports:
      - "4723:4723"

volumes:
  usbmuxd-socket:
    external: true
```

### Device Farm

```yaml
services:
  device-manager:
    image: your-device-manager
    volumes:
      - usbmuxd-socket:/var/run/usbmuxd
    environment:
      - USBMUXD_SOCKET=/var/run/usbmuxd

volumes:
  usbmuxd-socket:
    external: true
```

### Quick Device Check

```bash
# Check if devices are accessible
docker run --rm -v usbmuxd-socket:/var/run/usbmuxd \
  ghcr.io/danielpaulus/go-ios:latest list

# Get device info
docker run --rm -v usbmuxd-socket:/var/run/usbmuxd \
  ghcr.io/danielpaulus/go-ios:latest info
```

## Troubleshooting

### "Socket not found" in container

1. Verify the host relay is running
2. Check the extension backend status: `curl http://localhost:8080/status`
3. Ensure the volume is mounted correctly

### "Connection refused" on host relay

1. Check if usbmuxd is running on the host: `ls -la /var/run/usbmuxd`
2. On macOS, connect an iOS device or start Xcode
3. On Linux, ensure usbmuxd service is running: `systemctl status usbmuxd`

### No devices showing up

1. Verify the device is connected and trusted
2. Check host usbmuxd directly: `idevice_id -l` (on host)
3. View relay logs for connection issues

### Backend not connecting

1. Ensure host relay is running on port 27015
2. Check Docker Desktop can reach `host.docker.internal`
3. View backend logs: `docker logs <container-id>`

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

1. **Host Relay** (`usbmuxd-relay`): A Go binary that runs on the host, listens on TCP port 27015, and forwards connections to the real `/var/run/usbmuxd` socket.

2. **Backend Service**: Runs inside the Docker Desktop VM, connects to the host relay via `host.docker.internal:27015`, and creates a Unix socket at `/run/usbmuxd-shared/usbmuxd`.

3. **Shared Volume**: The `usbmuxd-socket` Docker volume contains the fake usbmuxd socket, which containers mount at `/var/run/usbmuxd`.

4. **Protocol Multiplexing**: Multiple container connections are multiplexed over the single TCP connection to the host, with each connection getting a unique ID.

## License

MIT
