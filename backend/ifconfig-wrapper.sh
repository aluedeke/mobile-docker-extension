#!/bin/sh
# Wrapper script that translates macOS-style ifconfig commands to Linux ip commands
# go-ios uses: ifconfig <iface> inet6 add <addr>/<prefix>
#              ifconfig <iface> mtu <value> up
#              ifconfig <iface> up

IFACE="$1"
shift

case "$1" in
    inet6)
        # ifconfig <iface> inet6 add <addr>/<prefix>
        # -> ip addr add <addr>/<prefix> dev <iface>
        if [ "$2" = "add" ]; then
            exec ip addr add "$3" dev "$IFACE"
        fi
        ;;
    mtu)
        # ifconfig <iface> mtu <value> up
        # -> ip link set <iface> mtu <value> up
        exec ip link set "$IFACE" mtu "$2" up
        ;;
    up)
        # ifconfig <iface> up
        # -> ip link set <iface> up
        exec ip link set "$IFACE" up
        ;;
esac

# Fallback to real ifconfig if available
exec /sbin/ifconfig.real "$IFACE" "$@" 2>/dev/null || echo "Unknown ifconfig command: $IFACE $@" >&2
