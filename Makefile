IMAGE_NAME ?= aluedeke/usbmuxd-docker-extension
TAG ?= latest

.PHONY: build install uninstall debug clean test-relay

# Build the extension image
build:
	docker build -t $(IMAGE_NAME):$(TAG) .

# Install the extension into Docker Desktop
install: build
	docker extension install $(IMAGE_NAME):$(TAG)

# Update the extension (faster than uninstall + install)
update: build
	docker extension update $(IMAGE_NAME):$(TAG)

# Uninstall the extension
uninstall:
	docker extension uninstall $(IMAGE_NAME):$(TAG)

# Enable debug mode for development
debug:
	docker extension dev debug $(IMAGE_NAME):$(TAG)

# Reset debug mode
reset:
	docker extension dev reset $(IMAGE_NAME):$(TAG)

# Build host binary locally (for testing)
build-host:
	cd host && go build -o ../bin/usbmuxd-relay main.go
	codesign -s - bin/usbmuxd-relay 2>/dev/null || true

# Build backend locally (for testing)
build-backend:
	cd backend && go build -o ../bin/backend main.go

# Run host relay locally for testing
run-host: build-host
	./bin/usbmuxd-relay -port 27015

# Install auto-start (macOS LaunchAgent)
autostart-install:
	@./scripts/install-autostart.sh

# Uninstall auto-start
autostart-uninstall:
	@./scripts/uninstall-autostart.sh

# Check auto-start status
autostart-status:
	@launchctl list | grep usbmuxd || echo "Not running"
	@[ -f ~/Library/LaunchAgents/com.docker.usbmuxd-relay.plist ] && echo "LaunchAgent: installed" || echo "LaunchAgent: not installed"

# Test the socket connection
test-socket:
	@echo "Testing connection to real usbmuxd..."
	@python3 -c "import socket; s=socket.socket(socket.AF_UNIX); s.connect('/var/run/usbmuxd'); print('Connected to usbmuxd'); s.close()"

# Clean build artifacts
clean:
	rm -rf bin/
	docker rmi $(IMAGE_NAME):$(TAG) 2>/dev/null || true

# View extension logs
logs:
	docker extension logs $(IMAGE_NAME):$(TAG)

# Open UI in debug mode
ui:
	docker extension dev ui-source $(IMAGE_NAME):$(TAG) http://localhost:3000

# Show help
help:
	@echo "USB Muxd Docker Extension"
	@echo ""
	@echo "Usage:"
	@echo "  make build      - Build the extension image"
	@echo "  make install    - Install the extension"
	@echo "  make update     - Update installed extension"
	@echo "  make uninstall  - Uninstall the extension"
	@echo "  make debug      - Enable debug mode"
	@echo "  make logs       - View extension logs"
	@echo ""
	@echo "Development:"
	@echo "  make build-host    - Build host binary locally"
	@echo "  make build-backend - Build backend locally"
	@echo "  make run-host      - Run host relay for testing"
	@echo "  make test-socket   - Test usbmuxd socket connection"
	@echo "  make clean         - Clean build artifacts"
	@echo ""
	@echo "Auto-start (macOS):"
	@echo "  make autostart-install   - Install LaunchAgent for auto-start"
	@echo "  make autostart-uninstall - Remove LaunchAgent"
	@echo "  make autostart-status    - Check auto-start status"
