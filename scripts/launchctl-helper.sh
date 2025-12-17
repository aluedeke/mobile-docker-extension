#!/bin/bash
# LaunchAgent helper for usbmuxd-relay on macOS
# Usage: launchctl-helper.sh <command>
# Commands: install, uninstall, status, running, start, stop, logs, clear-logs

set -e

LABEL="com.docker.usbmuxd-relay"
EXT_BASE="$HOME/Library/Containers/com.docker.docker/Data/extensions/aluedeke_usbmuxd-docker-extension"
RELAY_PATH="$EXT_BASE/host/usbmuxd-relay"
PLIST_PATH="$HOME/Library/LaunchAgents/$LABEL.plist"

cmd_install() {
    # Check if relay exists
    if [ ! -f "$RELAY_PATH" ]; then
        echo "Error: usbmuxd-relay not found at $RELAY_PATH"
        echo "Make sure the Docker extension is installed first."
        exit 1
    fi

    # Remove quarantine attribute (if downloaded from internet)
    xattr -d com.apple.quarantine "$RELAY_PATH" 2>/dev/null || true

    # Ad-hoc code sign the binary
    codesign -s - -f "$RELAY_PATH" 2>/dev/null || true

    # Create LaunchAgent plist for relay (just usbmuxd forwarding)
    # Bind to 127.0.0.1 so Docker VM can reach it via host.docker.internal
    cat > "$PLIST_PATH" << EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>$LABEL</string>
    <key>ProgramArguments</key>
    <array>
        <string>$RELAY_PATH</string>
        <string>-port</string>
        <string>27015</string>
        <string>-addr</string>
        <string>127.0.0.1</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>/tmp/usbmuxd-relay.log</string>
    <key>StandardErrorPath</key>
    <string>/tmp/usbmuxd-relay.log</string>
</dict>
</plist>
EOF

    # Load the relay agent
    launchctl unload "$PLIST_PATH" 2>/dev/null || true
    launchctl load "$PLIST_PATH"

    echo "installed"
}

cmd_uninstall() {
    # Unload and remove relay LaunchAgent
    if [ -f "$PLIST_PATH" ]; then
        launchctl unload "$PLIST_PATH" 2>/dev/null || true
        rm "$PLIST_PATH"
    fi

    # Kill any running relay
    pkill -f usbmuxd-relay 2>/dev/null || true

    echo "uninstalled"
}

cmd_status() {
    if launchctl list "$LABEL" >/dev/null 2>&1; then
        echo "installed"
    else
        echo "not_installed"
    fi
}

cmd_running() {
    # Check if relay process is actually running
    if pgrep -f usbmuxd-relay >/dev/null 2>&1; then
        echo "running"
    else
        echo "stopped"
    fi
}

LOG_FILE="/tmp/usbmuxd-relay.log"
PID_FILE="/tmp/usbmuxd-relay.pid"

cmd_start() {
    # Start relay manually (without LaunchAgent)
    # Check if already running by process name (more reliable than PID file)
    if pgrep -f usbmuxd-relay >/dev/null 2>&1; then
        echo "already_running"
        return
    fi

    # Clean up any stale PID file
    rm -f "$PID_FILE"

    # Start relay in background
    # The relay binary manages its own PID file
    # Bind to 127.0.0.1 so Docker VM can reach it via host.docker.internal
    nohup "$RELAY_PATH" -port 27015 -addr 127.0.0.1 >> "$LOG_FILE" 2>&1 &

    # Wait briefly for relay to start and write its PID file
    sleep 1

    if pgrep -f usbmuxd-relay >/dev/null 2>&1; then
        echo "started"
    else
        echo "failed"
    fi
}

cmd_stop() {
    # Stop relay
    if [ -f "$PID_FILE" ]; then
        pid=$(cat "$PID_FILE")
        kill "$pid" 2>/dev/null || true
        rm -f "$PID_FILE"
    fi

    # Also kill any running relay process by name
    pkill -f usbmuxd-relay 2>/dev/null || true

    echo "stopped"
}

cmd_logs() {
    if [ -f "$LOG_FILE" ]; then
        tail -100 "$LOG_FILE"
    fi
}

cmd_clear_logs() {
    if [ -f "$LOG_FILE" ]; then
        > "$LOG_FILE"
    fi
    echo "cleared"
}

# Main
case "${1:-}" in
    install)
        cmd_install
        ;;
    uninstall)
        cmd_uninstall
        ;;
    status)
        cmd_status
        ;;
    running)
        cmd_running
        ;;
    start)
        cmd_start
        ;;
    stop)
        cmd_stop
        ;;
    logs)
        cmd_logs
        ;;
    clear-logs)
        cmd_clear_logs
        ;;
    *)
        echo "Usage: $0 {install|uninstall|status|running|start|stop|logs|clear-logs}"
        exit 1
        ;;
esac
