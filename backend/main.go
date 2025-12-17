// backend/main.go
// USB Muxd backend - runs in Docker VM, creates fake usbmuxd socket for containers
package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"flag"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

var (
	hostAddr     = flag.String("host", "host.docker.internal:27015", "Host relay address")
	usbmuxdPath  = flag.String("usbmuxd", "/run/usbmuxd-shared/usbmuxd", "Path to create usbmuxd socket")
	socketPath   = flag.String("socket", "/run/guest-services/backend.sock", "Unix socket for API")
	enableTunnel = flag.Bool("tunnel", true, "Enable iOS 17.4+ tunnel manager (creates kernel TUN devices)")
)

// Tunnel manager instance (for iOS 17.4+ devices)
var tunnelManager *TunnelManager

type backend struct {
	mu       sync.RWMutex
	conns    map[uint32]net.Conn
	hostConn net.Conn
	hostMu   sync.Mutex
	nextID   uint32
	stats    struct {
		totalConns   uint64
		activeConns  int32
		bytesRelayed uint64
		connected    bool
		lastError    string
		connectedAt  time.Time
	}
}

func newBackend() *backend {
	return &backend{
		conns:  make(map[uint32]net.Conn),
		nextID: 1,
	}
}

func (b *backend) connectToHost() error {
	b.hostMu.Lock()
	defer b.hostMu.Unlock()

	if b.hostConn != nil {
		b.hostConn.Close()
	}

	log.Printf("Connecting to host relay at %s...", *hostAddr)

	conn, err := net.DialTimeout("tcp", *hostAddr, 10*time.Second)
	if err != nil {
		b.stats.lastError = err.Error()
		b.stats.connected = false
		return err
	}

	b.hostConn = conn
	b.stats.connected = true
	b.stats.lastError = ""
	b.stats.connectedAt = time.Now()
	log.Printf("Connected to host relay")

	return nil
}

func (b *backend) readFromHost() {
	header := make([]byte, 8)
	for {
		b.hostMu.Lock()
		conn := b.hostConn
		b.hostMu.Unlock()

		if conn == nil {
			time.Sleep(time.Second)
			continue
		}

		if _, err := io.ReadFull(conn, header); err != nil {
			log.Printf("Host read error: %v", err)
			b.stats.connected = false
			b.stats.lastError = err.Error()
			b.reconnectLoop()
			continue
		}

		connID := binary.BigEndian.Uint32(header[0:4])
		length := binary.BigEndian.Uint32(header[4:8])

		if length == 0 {
			// Connection closed by host
			b.mu.RLock()
			localConn, exists := b.conns[connID]
			b.mu.RUnlock()

			if exists {
				log.Printf("Connection %d closed by host", connID)
				localConn.Close()
				b.mu.Lock()
				delete(b.conns, connID)
				b.mu.Unlock()
				atomic.AddInt32(&b.stats.activeConns, -1)
			}
			continue
		}

		// Data from host
		payload := make([]byte, length)
		if _, err := io.ReadFull(conn, payload); err != nil {
			log.Printf("Host payload read error: %v", err)
			b.stats.connected = false
			b.stats.lastError = err.Error()
			b.reconnectLoop()
			continue
		}

		atomic.AddUint64(&b.stats.bytesRelayed, uint64(length))

		b.mu.RLock()
		localConn, ok := b.conns[connID]
		b.mu.RUnlock()

		if ok {
			if _, err := localConn.Write(payload); err != nil {
				log.Printf("Connection %d write error: %v", connID, err)
				localConn.Close()
				b.mu.Lock()
				delete(b.conns, connID)
				b.mu.Unlock()
				atomic.AddInt32(&b.stats.activeConns, -1)
			}
		}
	}
}

func (b *backend) reconnectLoop() {
	b.hostMu.Lock()
	if b.hostConn != nil {
		b.hostConn.Close()
		b.hostConn = nil
	}
	b.hostMu.Unlock()

	// Close all existing connections
	b.mu.Lock()
	for id, conn := range b.conns {
		conn.Close()
		delete(b.conns, id)
	}
	b.mu.Unlock()

	for {
		if err := b.connectToHost(); err != nil {
			log.Printf("Reconnect failed: %v, retrying in 5s...", err)
			time.Sleep(5 * time.Second)
			continue
		}
		break
	}
}

func (b *backend) handleLocalConnection(conn net.Conn) {
	b.mu.Lock()
	id := b.nextID
	b.nextID++
	b.conns[id] = conn
	b.mu.Unlock()

	atomic.AddUint64(&b.stats.totalConns, 1)
	atomic.AddInt32(&b.stats.activeConns, 1)
	log.Printf("New local connection %d from %s", id, conn.RemoteAddr())

	// Signal new connection to host
	b.sendToHost(id, nil)

	// Read from local connection, forward to host
	defer func() {
		conn.Close()
		b.mu.Lock()
		delete(b.conns, id)
		b.mu.Unlock()
		atomic.AddInt32(&b.stats.activeConns, -1)

		// Signal close to host
		b.sendToHost(id, nil)
		log.Printf("Connection %d closed", id)
	}()

	buf := make([]byte, 65536)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			if err != io.EOF {
				log.Printf("Connection %d read error: %v", id, err)
			}
			return
		}

		atomic.AddUint64(&b.stats.bytesRelayed, uint64(n))
		b.sendToHost(id, buf[:n])
	}
}

func (b *backend) sendToHost(connID uint32, data []byte) {
	header := make([]byte, 8)
	binary.BigEndian.PutUint32(header[0:4], connID)
	binary.BigEndian.PutUint32(header[4:8], uint32(len(data)))

	b.hostMu.Lock()
	defer b.hostMu.Unlock()

	if b.hostConn == nil {
		return
	}

	b.hostConn.Write(header)
	if len(data) > 0 {
		b.hostConn.Write(data)
	}
}

// checkHostConnection does a quick TCP dial to see if host relay is reachable
func (b *backend) checkHostConnection() bool {
	conn, err := net.DialTimeout("tcp", *hostAddr, 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// startTunnelInfoAPI starts an HTTP server compatible with go-ios tunnel info API
// This allows go-ios CLI to discover tunnels managed by our backend
func startTunnelInfoAPI() {
	mux := http.NewServeMux()

	// go-ios compatible endpoints
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		if tunnelManager != nil && tunnelManager.FirstUpdateCompleted() {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	})

	// GET /tunnel/{UDID} - get tunnel info for specific device
	mux.HandleFunc("/tunnel/", func(w http.ResponseWriter, r *http.Request) {
		if tunnelManager == nil {
			http.Error(w, "tunnel manager not enabled", http.StatusServiceUnavailable)
			return
		}

		udid := r.URL.Path[len("/tunnel/"):]
		if udid == "" {
			http.Error(w, "missing udid", http.StatusBadRequest)
			return
		}

		t, err := tunnelManager.FindTunnel(udid)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if t.Udid == "" {
			http.Error(w, "tunnel not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(t)
	})

	// GET /tunnels - list all tunnels
	mux.HandleFunc("/tunnels", func(w http.ResponseWriter, r *http.Request) {
		if tunnelManager == nil {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]interface{}{})
			return
		}

		tunnels, err := tunnelManager.ListTunnels()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tunnels)
	})

	// Listen on port 60105 (go-ios default)
	addr := "127.0.0.1:60105"
	log.Printf("Tunnel info API listening on %s (go-ios compatible)", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Printf("Tunnel info API error: %v", err)
	}
}

func (b *backend) startAPI() {
	mux := http.NewServeMux()

	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Do an active check to see if host relay is reachable
		hostReachable := b.checkHostConnection()

		status := map[string]interface{}{
			"hostReachable": hostReachable,
			"connected":     b.stats.connected,
			"totalConns":    atomic.LoadUint64(&b.stats.totalConns),
			"activeConns":   atomic.LoadInt32(&b.stats.activeConns),
			"bytesRelayed":  atomic.LoadUint64(&b.stats.bytesRelayed),
			"lastError":     b.stats.lastError,
			"hostAddress":   *hostAddr,
			"socketPath":    *usbmuxdPath,
		}
		if b.stats.connected {
			status["connectedFor"] = time.Since(b.stats.connectedAt).String()
		}
		json.NewEncoder(w).Encode(status)
	})

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if b.stats.connected {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("OK"))
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("Not connected to host"))
		}
	})

	// Tunnel manager endpoints (iOS 17.4+ devices) - for extension UI
	mux.HandleFunc("/tunnel/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if tunnelManager == nil {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"enabled": false,
			})
			return
		}
		tunnels, _ := tunnelManager.ListTunnels()
		json.NewEncoder(w).Encode(map[string]interface{}{
			"enabled":     true,
			"tunnelCount": len(tunnels),
			"tunnels":     tunnels,
		})
	})

	// Remove existing socket file
	os.RemoveAll(*socketPath)

	// Listen on Unix socket
	listener, err := net.Listen("unix", *socketPath)
	if err != nil {
		log.Fatalf("Failed to listen on socket %s: %v", *socketPath, err)
	}
	log.Printf("API server listening on Unix socket %s", *socketPath)
	http.Serve(listener, mux)
}

func main() {
	flag.Parse()
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	b := newBackend()

	// Remove stale socket
	os.Remove(*usbmuxdPath)

	// Ensure directory exists
	dir := "/run/usbmuxd-shared"
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Fatalf("Failed to create directory %s: %v", dir, err)
	}

	// Write environment file for containers to source
	envContent := `# usbmuxd-relay environment variables
# Source this file: . /var/run/usbmuxd/usbmuxd.env
# Or use --env-file: docker run --env-file <(docker run --rm -v usbmuxd-socket:/mnt alpine cat /mnt/usbmuxd.env) ...
USBMUXD_SOCKET_ADDRESS=host.docker.internal:27015
TUNNEL_INFO_URL=http://host.docker.internal:28100
TUNNEL_INFO_PORT=28100
`
	envPath := dir + "/usbmuxd.env"
	if err := os.WriteFile(envPath, []byte(envContent), 0644); err != nil {
		log.Printf("Warning: couldn't write env file: %v", err)
	} else {
		log.Printf("Environment file written to %s", envPath)
	}

	// Start tunnel manager if enabled (for iOS 17.4+ devices)
	// Creates kernel TUN devices for each connected iOS 17.4+ device
	if *enableTunnel {
		tunnelManager = NewTunnelManager()
		tunnelManager.Start(context.Background())
		log.Printf("Tunnel manager enabled for iOS 17.4+ devices (kernel TUN)")

		// Start go-ios compatible tunnel info API on port 60105
		// This allows go-ios CLI to discover our tunnels
		go startTunnelInfoAPI()
	}

	// Start API server
	go b.startAPI()

	// Connect to host relay (with retries)
	for {
		if err := b.connectToHost(); err != nil {
			log.Printf("Initial connection failed: %v, retrying in 5s...", err)
			time.Sleep(5 * time.Second)
			continue
		}
		break
	}

	// Start reading from host
	go b.readFromHost()

	// Create usbmuxd socket for containers
	listener, err := net.Listen("unix", *usbmuxdPath)
	if err != nil {
		log.Fatalf("Failed to create usbmuxd socket: %v", err)
	}
	defer listener.Close()

	// Make socket accessible to containers (they run as various UIDs)
	// This is safe because it's inside the Docker VM, not exposed to host
	if err := os.Chmod(*usbmuxdPath, 0666); err != nil {
		log.Printf("Warning: couldn't chmod socket: %v", err)
	}

	log.Printf("USB Muxd socket created at %s", *usbmuxdPath)
	log.Printf("Containers can mount volume 'usbmuxd-socket' at /var/run/usbmuxd")

	// Accept connections from containers
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Accept error: %v", err)
			continue
		}

		go b.handleLocalConnection(conn)
	}
}
