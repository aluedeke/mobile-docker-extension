#!/bin/sh
# Entrypoint script for go-ios container
# Starts the tunnel agent in the background and waits for it to be ready

# Start tunnel agent in background if ENABLE_GO_IOS_AGENT is set
if [ "$ENABLE_GO_IOS_AGENT" = "kernel" ] || [ "$ENABLE_GO_IOS_AGENT" = "user" ]; then
    # Check if agent is already running
    if ! wget -q -O /dev/null http://127.0.0.1:60105/health 2>/dev/null; then
        echo "Starting go-ios tunnel agent..."
        if [ "$ENABLE_GO_IOS_AGENT" = "kernel" ]; then
            ios tunnel start &
        else
            ios tunnel start --userspace &
        fi

        # Wait for agent to be ready (up to 30 seconds)
        echo "Waiting for tunnel agent to be ready..."
        for i in $(seq 1 30); do
            if wget -q -O /dev/null http://127.0.0.1:60105/ready 2>/dev/null; then
                echo "Tunnel agent is ready"
                break
            fi
            sleep 1
        done
    fi
fi

# Unset ENABLE_GO_IOS_AGENT so ios doesn't try to start another agent
unset ENABLE_GO_IOS_AGENT

# Execute the actual command
exec ios "$@"
