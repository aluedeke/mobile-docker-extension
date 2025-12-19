#!/bin/bash
# LaunchAgent helper for mobile-relay on macOS
# The relay binary now includes iOS 17.4+ tunnel management
# Usage: launchctl-helper.sh <command>
# Commands: install, uninstall, status, running, start, stop, logs, clear-logs

set -e

RELAY_LABEL="com.docker.mobile-relay"
EXT_BASE="$HOME/Library/Containers/com.docker.docker/Data/extensions/aluedeke_usbmuxd-docker-extension"
RELAY_PATH="$EXT_BASE/host/mobile-relay"
RELAY_PLIST="$HOME/Library/LaunchAgents/$RELAY_LABEL.plist"

LOG_FILE="/tmp/mobile-relay.log"
PID_FILE="/tmp/mobile-relay.pid"

cmd_install() {
    # Check if relay exists
    if [ ! -f "$RELAY_PATH" ]; then
        echo "Error: mobile-relay not found at $RELAY_PATH"
        echo "Make sure the Docker extension is installed first."
        exit 1
    fi

    # Remove quarantine attribute (if downloaded from internet)
    xattr -d com.apple.quarantine "$RELAY_PATH" 2>/dev/null || true

    # Ad-hoc code sign the binary
    codesign -s - -f "$RELAY_PATH" 2>/dev/null || true

    # Create LaunchAgent plist for relay (includes tunnel manager)
    cat > "$RELAY_PLIST" << EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>$RELAY_LABEL</string>
    <key>ProgramArguments</key>
    <array>
        <string>$RELAY_PATH</string>
        <string>-port</string>
        <string>27015</string>
        <string>-addr</string>
        <string>127.0.0.1</string>
        <string>-tunnel-port</string>
        <string>60105</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>$LOG_FILE</string>
    <key>StandardErrorPath</key>
    <string>$LOG_FILE</string>
</dict>
</plist>
EOF

    # Load the agent
    launchctl unload "$RELAY_PLIST" 2>/dev/null || true
    launchctl load "$RELAY_PLIST"

    echo "installed"
}

cmd_uninstall() {
    # Unload and remove relay LaunchAgent
    if [ -f "$RELAY_PLIST" ]; then
        launchctl unload "$RELAY_PLIST" 2>/dev/null || true
        rm "$RELAY_PLIST"
    fi

    # Kill any running processes
    pkill -f mobile-relay 2>/dev/null || true

    echo "uninstalled"
}

cmd_status() {
    if launchctl list "$RELAY_LABEL" >/dev/null 2>&1; then
        echo "installed"
    else
        echo "not_installed"
    fi
}

cmd_running() {
    # Check if relay process is actually running
    if pgrep -f mobile-relay >/dev/null 2>&1; then
        echo "running"
    else
        echo "stopped"
    fi
}

cmd_start() {
    # Start relay manually (without LaunchAgent)
    if pgrep -f mobile-relay >/dev/null 2>&1; then
        echo "already_running"
        return
    fi

    # Clean up any stale PID files
    rm -f "$PID_FILE"

    # Start relay in background (includes tunnel manager)
    nohup "$RELAY_PATH" -port 27015 -addr 127.0.0.1 -tunnel-port 60105 >> "$LOG_FILE" 2>&1 &

    # Wait briefly for process to start
    sleep 1

    if pgrep -f mobile-relay >/dev/null 2>&1; then
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

    # Also kill any running processes by name
    pkill -f mobile-relay 2>/dev/null || true

    echo "stopped"
}

cmd_logs() {
    echo "=== mobile-relay logs (includes tunnel manager) ==="
    if [ -f "$LOG_FILE" ]; then
        tail -50 "$LOG_FILE"
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
