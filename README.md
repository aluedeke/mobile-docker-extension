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
│                  │ ios-tunnel  │ (manages iOS 17.4+ tunnels)    │
│                  │  backend    │                                │
│                  └──────┬──────┘                                │
│                         │ TUN interfaces                        │
│  ┌──────────────────────┼──────────────────────────────────────┐│
│  │ Your Container       │ (network_mode: container:ios-tunnel) ││
│  │                      ↓                                      ││
│  │   USBMUXD_SOCKET_ADDRESS=host.docker.internal:27015         ││
│  │                                                             ││
│  │   go-ios, libimobiledevice, idevice_id, etc.               ││
│  └─────────────────────────────────────────────────────────────┘│
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

### 2. Configure your containers

Set the `USBMUXD_SOCKET_ADDRESS` environment variable to connect to the host relay:

**Docker run:**
```bash
docker run -e USBMUXD_SOCKET_ADDRESS=host.docker.internal:27015 your-image
```

**Docker Compose:**
```yaml
services:
  ios-automation:
    image: your-image
    environment:
      - USBMUXD_SOCKET_ADDRESS=host.docker.internal:27015
```

### 3. For iOS 17.4+ features (tunnels)

iOS 17.4+ requires kernel TUN tunnels for many features. The extension's backend container (`ios-tunnel`) manages these automatically. To access them, share its network namespace:

```yaml
services:
  ios-automation:
    image: your-image
    network_mode: "container:ios-tunnel"
    environment:
      - USBMUXD_SOCKET_ADDRESS=host.docker.internal:27015
```

### 4. Use iOS tools normally

Once configured, iOS tools work as if running on the host:

```bash
# List connected devices with go-ios
docker run --rm -e USBMUXD_SOCKET_ADDRESS=host.docker.internal:27015 \
  ghcr.io/danielpaulus/go-ios:latest list

# Use libimobiledevice tools
docker run --rm -e USBMUXD_SOCKET_ADDRESS=host.docker.internal:27015 \
  alpine sh -c "apk add libimobiledevice && idevice_id -l"
```

## Examples

See the [examples/go-ios](examples/go-ios) directory for a complete example using go-ios.

### iOS Test Automation

```yaml
# docker-compose.yaml
services:
  appium:
    image: appium/appium
    network_mode: "container:ios-tunnel"
    environment:
      - USBMUXD_SOCKET_ADDRESS=host.docker.internal:27015
    ports:
      - "4723:4723"
```

### Device Farm

```yaml
services:
  device-manager:
    image: your-device-manager
    network_mode: "container:ios-tunnel"
    environment:
      - USBMUXD_SOCKET_ADDRESS=host.docker.internal:27015
```

### Quick Device Check

```bash
# Check if devices are accessible
docker run --rm -e USBMUXD_SOCKET_ADDRESS=host.docker.internal:27015 \
  ghcr.io/danielpaulus/go-ios:latest list

# Get device info
docker run --rm -e USBMUXD_SOCKET_ADDRESS=host.docker.internal:27015 \
  ghcr.io/danielpaulus/go-ios:latest info
```

## Troubleshooting

### "Connection refused" when connecting to usbmuxd

1. Verify the host relay is running: `pgrep usbmuxd-relay`
2. Check the relay is listening: `lsof -i :27015`
3. Ensure the `USBMUXD_SOCKET_ADDRESS` environment variable is set correctly

### "Connection refused" on host relay

1. Check if usbmuxd is running on the host: `ls -la /var/run/usbmuxd`
2. On macOS, connect an iOS device or start Xcode
3. On Linux, ensure usbmuxd service is running: `systemctl status usbmuxd`

### No devices showing up

1. Verify the device is connected and trusted
2. Check host usbmuxd directly: `idevice_id -l` (on host)
3. View relay logs for connection issues

### iOS 17.4+ tunnel features not working

1. Ensure your container uses `network_mode: "container:ios-tunnel"`
2. Check the ios-tunnel backend is running: `docker ps | grep ios-tunnel`
3. View backend logs: `docker logs ios-tunnel`

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

1. **Host Relay** (`usbmuxd-relay`): A Go binary that runs on the host, listens on TCP port 27015, and forwards connections to the real `/var/run/usbmuxd` socket. Each TCP connection gets its own usbmuxd connection.

2. **Backend Service** (`ios-tunnel`): Runs inside the Docker Desktop VM. Manages iOS 17.4+ kernel TUN tunnels automatically. Provides a tunnel info API on port 60105 (go-ios compatible).

3. **Container Configuration**: Containers connect to usbmuxd via `USBMUXD_SOCKET_ADDRESS=host.docker.internal:27015`. For iOS 17.4+ tunnel access, they share the network namespace with `ios-tunnel`.

## License

MIT
