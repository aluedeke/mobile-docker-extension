#!/bin/bash
set -e

echo "=== USB Muxd Docker Extension Test ==="

# Check if usbmuxd is available on host
if [ ! -S /var/run/usbmuxd ]; then
    echo "ERROR: /var/run/usbmuxd socket not found"
    echo "Make sure an iOS device is connected or usbmuxd is running"
    exit 1
fi

echo "[1/4] Building extension image..."
docker build -t usbmuxd-test .

echo "[2/4] Building host relay..."
mkdir -p bin
cd host && go build -o ../bin/usbmuxd-relay main.go && cd ..

echo "[3/4] Starting host relay (background)..."
./bin/usbmuxd-relay -port 27015 &
HOST_PID=$!
sleep 1

cleanup() {
    echo "Cleaning up..."
    kill $HOST_PID 2>/dev/null || true
    docker rm -f usbmuxd-backend-test 2>/dev/null || true
}
trap cleanup EXIT

echo "[4/4] Starting backend container..."
docker run -d --name usbmuxd-backend-test \
    -v usbmuxd-socket:/run/usbmuxd-shared \
    usbmuxd-test /backend -host host.docker.internal:27015

sleep 2

echo ""
echo "=== Testing Connection ==="

# Check backend status
echo "Backend status:"
docker exec usbmuxd-backend-test wget -qO- http://localhost:8080/status | python3 -m json.tool 2>/dev/null || echo "(no python3, raw output)"

echo ""
echo "=== Testing usbmuxd Protocol ==="

# Try to list devices using a simple test
docker run --rm -v usbmuxd-socket:/var/run/usbmuxd alpine sh -c '
    # Send a minimal usbmuxd "list devices" request
    echo "Attempting to connect to usbmuxd socket..."
    if [ -S /var/run/usbmuxd ]; then
        echo "Socket exists!"
        ls -la /var/run/usbmuxd
    else
        echo "Socket not found"
        ls -la /var/run/
    fi
'

echo ""
echo "=== Test with go-ios (if available) ==="
docker run --rm -v usbmuxd-socket:/var/run/usbmuxd \
    ghcr.io/danielpaulus/go-ios:latest list 2>&1 || echo "(go-ios test complete)"

echo ""
echo "=== Test Complete ==="
echo "Backend logs:"
docker logs usbmuxd-backend-test 2>&1 | tail -20
