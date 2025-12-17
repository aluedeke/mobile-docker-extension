// backend/tunnel_manager.go
// Tunnel manager for iOS 17.4+ devices using go-ios kernel TUN tunnel
package main

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/Masterminds/semver"
	"github.com/danielpaulus/go-ios/ios"
	"github.com/danielpaulus/go-ios/ios/tunnel"
)

// minIOSVersionForTunnel is the minimum iOS version that requires tunnel
// and supports ConnectTunnelLockdown (which doesn't require filesystem pair records)
var minIOSVersionForTunnel = semver.MustParse("17.4.0")

// TunnelManager manages iOS 17.4+ device tunnels using go-ios
type TunnelManager struct {
	mu             sync.RWMutex
	tunnels        map[string]tunnel.Tunnel
	updateInterval time.Duration
	ctx            context.Context
	cancel         context.CancelFunc
	firstUpdate    bool
	firstUpdateMu  sync.Mutex
}

// NewTunnelManager creates a new tunnel manager
func NewTunnelManager() *TunnelManager {
	return &TunnelManager{
		tunnels:        make(map[string]tunnel.Tunnel),
		updateInterval: 2 * time.Second,
	}
}

// Start begins the tunnel manager background loop
func (m *TunnelManager) Start(ctx context.Context) {
	m.ctx, m.cancel = context.WithCancel(ctx)
	go m.updateLoop()
}

// Stop shuts down all tunnels and stops the manager
func (m *TunnelManager) Stop() {
	if m.cancel != nil {
		m.cancel()
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for udid, t := range m.tunnels {
		log.Printf("Stopping tunnel for device %s", udid)
		if err := t.Close(); err != nil {
			log.Printf("Error closing tunnel for %s: %v", udid, err)
		}
		delete(m.tunnels, udid)
	}
}

// updateLoop periodically checks for device changes and updates tunnels
func (m *TunnelManager) updateLoop() {
	// Do first update immediately
	m.updateTunnels()

	ticker := time.NewTicker(m.updateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.updateTunnels()
		}
	}
}

// updateTunnels discovers devices and starts/stops tunnels as needed
func (m *TunnelManager) updateTunnels() {
	devices, err := ios.ListDevices()
	if err != nil {
		log.Printf("TunnelManager: failed to list devices: %v", err)
		return
	}

	m.mu.Lock()
	existingUdids := make(map[string]bool)
	for udid := range m.tunnels {
		existingUdids[udid] = true
	}
	m.mu.Unlock()

	// Start tunnels for new devices
	for _, device := range devices.DeviceList {
		udid := device.Properties.SerialNumber

		m.mu.RLock()
		_, exists := m.tunnels[udid]
		m.mu.RUnlock()

		if exists {
			delete(existingUdids, udid)
			continue
		}

		// Check iOS version
		version, err := ios.GetProductVersion(device)
		if err != nil {
			log.Printf("TunnelManager: failed to get iOS version for %s: %v", udid, err)
			continue
		}

		if version.LessThan(minIOSVersionForTunnel) {
			log.Printf("TunnelManager: device %s has iOS %s (< 17.4), no tunnel needed", udid, version)
			continue
		}

		// Start tunnel for this device
		m.startTunnelForDevice(device)
	}

	// Stop tunnels for disconnected devices
	for udid := range existingUdids {
		m.mu.Lock()
		if t, ok := m.tunnels[udid]; ok {
			log.Printf("TunnelManager: device %s disconnected, stopping tunnel", udid)
			t.Close()
			delete(m.tunnels, udid)
		}
		m.mu.Unlock()
	}

	// Mark first update complete
	m.firstUpdateMu.Lock()
	m.firstUpdate = true
	m.firstUpdateMu.Unlock()
}

// startTunnelForDevice starts a kernel TUN tunnel for the given device
func (m *TunnelManager) startTunnelForDevice(device ios.DeviceEntry) {
	udid := device.Properties.SerialNumber
	log.Printf("TunnelManager: starting kernel TUN tunnel for device %s", udid)

	// Start the kernel TUN tunnel using go-ios
	// This connects to CoreDeviceProxy via usbmuxd (our relayed socket)
	// and creates a real TUN device that any tool can use
	t, err := tunnel.ConnectTunnelLockdown(device)
	if err != nil {
		log.Printf("TunnelManager: failed to start tunnel for %s: %v", udid, err)
		return
	}

	m.mu.Lock()
	m.tunnels[udid] = t
	m.mu.Unlock()

	log.Printf("TunnelManager: kernel TUN tunnel started for %s (address: %s, rsd port: %d)", udid, t.Address, t.RsdPort)
}

// ListTunnels returns all active tunnels
func (m *TunnelManager) ListTunnels() ([]tunnel.Tunnel, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tunnels := make([]tunnel.Tunnel, 0, len(m.tunnels))
	for _, t := range m.tunnels {
		tunnels = append(tunnels, t)
	}
	return tunnels, nil
}

// FindTunnel finds a tunnel by device UDID
func (m *TunnelManager) FindTunnel(udid string) (tunnel.Tunnel, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if t, ok := m.tunnels[udid]; ok {
		return t, nil
	}
	return tunnel.Tunnel{}, nil
}

// FirstUpdateCompleted returns true if the first device scan is complete
func (m *TunnelManager) FirstUpdateCompleted() bool {
	m.firstUpdateMu.Lock()
	defer m.firstUpdateMu.Unlock()
	return m.firstUpdate
}
