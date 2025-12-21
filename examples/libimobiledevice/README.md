# libimobiledevice Example

This example demonstrates using [libimobiledevice](https://libimobiledevice.org/) with the mobile-docker-extension to interact with iOS devices from Docker containers.

## Prerequisites

1. Install the mobile-docker-extension in Docker Desktop
2. Start the host relay (via the extension UI or manually)
3. Connect an iOS device via USB

## Setup

Build the libimobiledevice image:

```bash
docker compose build
```

## Usage

### Device Discovery

```bash
# List connected devices (UDIDs)
docker compose run --rm libimobiledevice idevice_id -l

# List devices with network devices included
docker compose run --rm libimobiledevice idevice_id -l -n
```

### Device Information

```bash
# Get full device info
docker compose run --rm libimobiledevice ideviceinfo

# Get specific domain info
docker compose run --rm libimobiledevice ideviceinfo -q com.apple.disk_usage

# Get device name
docker compose run --rm libimobiledevice idevicename

# Set device name
docker compose run --rm libimobiledevice idevicename "My iPhone"
```

### System Log

```bash
# Stream system log
docker compose run --rm libimobiledevice idevicesyslog

# Stream with process filter
docker compose run --rm libimobiledevice idevicesyslog -p SpringBoard
```

### Screenshots

```bash
# Take a screenshot
docker compose run --rm libimobiledevice idevicescreenshot screenshot.png

# Save to stdout and redirect
docker compose run --rm libimobiledevice idevicescreenshot - > screenshot.png
```

### App Management

```bash
# List installed apps
docker compose run --rm libimobiledevice ideviceinstaller -l

# List user apps only
docker compose run --rm libimobiledevice ideviceinstaller -l -o list_user

# Install an app (mount the .ipa first)
docker compose run --rm -v ./app.ipa:/app.ipa libimobiledevice ideviceinstaller -i /app.ipa

# Uninstall an app
docker compose run --rm libimobiledevice ideviceinstaller -U com.example.app
```

### Device Pairing

```bash
# Pair with device (requires trust on device)
docker compose run --rm libimobiledevice idevicepair pair

# Validate pairing
docker compose run --rm libimobiledevice idevicepair validate

# Unpair device
docker compose run --rm libimobiledevice idevicepair unpair
```

### Diagnostics

```bash
# Restart device
docker compose run --rm libimobiledevice idevicediagnostics restart

# Shutdown device
docker compose run --rm libimobiledevice idevicediagnostics shutdown

# Sleep device
docker compose run --rm libimobiledevice idevicediagnostics sleep

# Get diagnostics info
docker compose run --rm libimobiledevice idevicediagnostics diagnostics All
```

### Backup & Restore

```bash
# Create a backup
docker compose run --rm -v ./backups:/backups libimobiledevice idevicebackup2 backup /backups

# Restore from backup
docker compose run --rm -v ./backups:/backups libimobiledevice idevicebackup2 restore /backups
```

### File System Access

```bash
# Browse app documents (via AFC)
docker compose run --rm libimobiledevice ifuse --documents com.example.app /mnt

# Access crash logs
docker compose run --rm libimobiledevice idevicecrashreport -e /crashes
```

### Date & Time

```bash
# Get device date
docker compose run --rm libimobiledevice idevicedate

# Set device date (requires supervision)
docker compose run --rm libimobiledevice idevicedate -s "2024-01-01 12:00:00"
```

## Available Commands

| Command | Description |
|---------|-------------|
| `idevice_id` | List attached devices |
| `ideviceinfo` | Show device information |
| `idevicename` | Get/set device name |
| `idevicesyslog` | Stream system log |
| `idevicescreenshot` | Take screenshots |
| `ideviceinstaller` | Manage apps |
| `idevicepair` | Manage device pairing |
| `idevicediagnostics` | Diagnostics and power management |
| `idevicebackup2` | Backup and restore |
| `idevicedate` | Get/set device date |
| `idevicedebug` | Debug utilities |
| `idevicecrashreport` | Retrieve crash reports |
| `ideviceprovision` | Manage provisioning profiles |
| `ideviceimagemounter` | Mount developer disk images |

## How It Works

The docker-compose.yaml mounts the usbmuxd socket from the extension directly into the container at `/var/run/usbmuxd`. libimobiledevice tools use this standard socket location automatically.
