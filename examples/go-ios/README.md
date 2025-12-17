# go-ios Example

This example shows how to use [go-ios](https://github.com/danielpaulus/go-ios) with the usbmuxd-docker-extension.

## Prerequisites

1. Install the usbmuxd-docker-extension in Docker Desktop
2. Start the host relay (via the extension UI or manually)
3. Connect an iOS device via USB

## Quick Start

```bash
# Build the go-ios image
docker compose build

# List connected devices
docker compose run --rm go-ios list

# Get device info
docker compose run --rm go-ios info
```

## iOS 17.4+ Devices

For iOS 17.4+ devices, go-ios automatically starts a kernel TUN tunnel when needed (via `ENABLE_GO_IOS_AGENT=kernel`):

```bash
# List active tunnels
docker compose run --rm go-ios tunnel ls

# Get display info (requires tunnel - uses CoreDevice XPC service)
docker compose run --rm go-ios info display

# View syslog (uses tunnel on iOS 17.4+)
docker compose run --rm go-ios syslog
```

## Common Commands

```bash
# List devices (usbmuxd)
docker compose run --rm go-ios list

# Device info (usbmuxd via Lockdown)
docker compose run --rm go-ios info

# List installed apps (usbmuxd)
docker compose run --rm go-ios apps

# Take a screenshot
docker compose run --rm go-ios screenshot --output /tmp/screen.png

# Install an app (mount the IPA file)
docker compose run --rm -v /path/to/app.ipa:/app.ipa go-ios install /app.ipa
```

## How It Works

The docker-compose.yaml configures the go-ios container to:

1. **Mount `usbmuxd-socket` volume**: Provides access to the relayed usbmuxd socket
2. **Set `USBMUXD_SOCKET_ADDRESS`**: Tells go-ios where to find the usbmuxd socket
3. **Set `ENABLE_GO_IOS_AGENT=kernel`**: Automatically starts the go-ios tunnel agent for iOS 17.4+ features
4. **Add `NET_ADMIN` capability and `/dev/net/tun`**: Required for creating kernel TUN devices

## Which Commands Need the Tunnel?

| Command | Tunnel Required | Notes |
|---------|----------------|-------|
| `list` | No | Uses usbmuxd |
| `info` | No | Uses Lockdown via usbmuxd |
| `info display` | **Yes** | Uses CoreDevice XPC service |
| `apps` | No | Uses installation_proxy via usbmuxd |
| `afc` | No | Uses AFC via usbmuxd |
| `syslog` | **Yes** (iOS 17.4+) | Uses shim service via tunnel |
| `instruments` | **Yes** (iOS 17.4+) | Uses DTX via tunnel |
