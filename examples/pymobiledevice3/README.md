# pymobiledevice3 Example

This example demonstrates using [pymobiledevice3](https://github.com/doronz88/pymobiledevice3) with the usbmuxd-docker-extension to interact with iOS devices from Docker containers.

## Prerequisites

1. Install the usbmuxd-docker-extension in Docker Desktop
2. Start the host relay (via the extension UI or manually)
3. Connect an iOS device via USB

## Setup

Build the pymobiledevice3 image:

```bash
docker compose build
```

## Usage

### Basic Commands (usbmuxd)

```bash
# List connected devices
docker compose run --rm pymobiledevice3 usbmux list

# Get all device info
docker compose run --rm pymobiledevice3 lockdown info

# Get specific device value
docker compose run --rm pymobiledevice3 lockdown get --key DeviceName

# Get device date
docker compose run --rm pymobiledevice3 lockdown date
```

### Tunnel-based Commands (iOS 17+)

pymobiledevice3 supports tunnels over usbmuxd, enabling access to developer services without additional configuration:

```bash
# Stream device syslog
docker compose run --rm pymobiledevice3 syslog live

# Process monitoring
docker compose run --rm pymobiledevice3 developer dvt sysmon process single

# List running apps
docker compose run --rm pymobiledevice3 developer dvt app-list

# Take a screenshot
docker compose run --rm pymobiledevice3 developer dvt screenshot /dev/stdout > screenshot.png
```

### Developer Services

```bash
# Mount developer disk image (required for some commands on older iOS)
docker compose run --rm pymobiledevice3 developer auto-mount

# Simulate location
docker compose run --rm pymobiledevice3 developer simulate-location set -- 37.7749 -122.4194

# Clear simulated location
docker compose run --rm pymobiledevice3 developer simulate-location clear
```

### Diagnostics

```bash
# Battery info
docker compose run --rm pymobiledevice3 lockdown get --domain com.apple.mobile.battery

# WiFi info
docker compose run --rm pymobiledevice3 lockdown get --domain com.apple.mobile.wireless_lockdown
```

## How It Works

The docker-compose.yaml mounts the usbmuxd socket from the extension directly into the container at `/var/run/usbmuxd`. pymobiledevice3 uses this standard socket location automatically.

For iOS 17+ devices, pymobiledevice3 negotiates tunnels transparently over the usbmuxd connection - no additional environment variables or tunnel services required.
