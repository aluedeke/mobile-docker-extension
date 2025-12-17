// host/main.go
// USB Muxd relay - runs on macOS host, bridges TCP to real usbmuxd socket
package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
)

var (
	usbmuxdPath = flag.String("usbmuxd", "/var/run/usbmuxd", "Path to real usbmuxd socket")
	listenPort  = flag.Int("port", 27015, "TCP port to listen on")
	listenAddr  = flag.String("addr", "127.0.0.1", "Address to listen on")
	pidFile     = flag.String("pidfile", "", "Path to PID file (default: /tmp/usbmuxd-relay.pid)")
	logFile     = flag.String("logfile", "", "Path to log file (default: stdout/stderr)")
)

// Default PID file location
func getPidFilePath() string {
	if *pidFile != "" {
		return *pidFile
	}
	return "/tmp/usbmuxd-relay.pid"
}

// Check if another instance is running by reading the PID file
func checkExistingProcess() (int, bool) {
	pidPath := getPidFilePath()
	data, err := os.ReadFile(pidPath)
	if err != nil {
		return 0, false
	}

	pidStr := strings.TrimSpace(string(data))
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		os.Remove(pidPath)
		return 0, false
	}

	// Check if process is still running
	process, err := os.FindProcess(pid)
	if err != nil {
		os.Remove(pidPath)
		return 0, false
	}

	// On Unix, FindProcess always succeeds - need to send signal 0 to check
	err = process.Signal(syscall.Signal(0))
	if err != nil {
		// Process doesn't exist, clean up stale PID file
		os.Remove(pidPath)
		return 0, false
	}

	// Process exists, but verify it's actually usbmuxd-relay (not a recycled PID)
	// Use ps to check the process name
	cmd := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "comm=")
	output, err := cmd.Output()
	if err != nil {
		// Can't verify, assume stale
		os.Remove(pidPath)
		return 0, false
	}

	procName := strings.TrimSpace(string(output))
	if !strings.Contains(procName, "usbmuxd-relay") {
		// Different process reused this PID, clean up stale file
		log.Printf("PID %d exists but is '%s', not usbmuxd-relay - removing stale PID file", pid, procName)
		os.Remove(pidPath)
		return 0, false
	}

	return pid, true
}

// Write our PID to the PID file
func writePidFile() error {
	pidPath := getPidFilePath()

	// Ensure directory exists
	dir := filepath.Dir(pidPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create PID file directory: %w", err)
	}

	pid := os.Getpid()
	return os.WriteFile(pidPath, []byte(strconv.Itoa(pid)), 0644)
}

// Remove the PID file
func removePidFile() {
	os.Remove(getPidFilePath())
}

// Protocol between host and backend:
// Each message: [4 bytes: connection ID][4 bytes: payload length][payload]
// - connID > 0, length = 0: new connection request / connection closed
// - connID > 0, length > 0: data for connection

type relay struct {
	mu       sync.RWMutex
	conns    map[uint32]net.Conn
	clientMu sync.Mutex
	client   net.Conn
	nextID   uint32
	stats    struct {
		totalConns   uint64
		activeConns  int32
		bytesRelayed uint64
	}
}

func newRelay() *relay {
	return &relay{
		conns: make(map[uint32]net.Conn),
	}
}

func (r *relay) handleClient(client net.Conn) {
	r.clientMu.Lock()
	if r.client != nil {
		r.client.Close()
	}
	r.client = client
	r.clientMu.Unlock()

	log.Printf("Backend connected from %s", client.RemoteAddr())

	// Clean up existing connections when a new backend connects
	r.mu.Lock()
	for id, conn := range r.conns {
		conn.Close()
		delete(r.conns, id)
	}
	r.mu.Unlock()

	// Read messages from backend
	header := make([]byte, 8)
	for {
		if _, err := io.ReadFull(client, header); err != nil {
			if err != io.EOF {
				log.Printf("Backend read error: %v", err)
			}
			break
		}

		connID := binary.BigEndian.Uint32(header[0:4])
		length := binary.BigEndian.Uint32(header[4:8])

		if length == 0 {
			// New connection request or close signal
			r.mu.RLock()
			existing, exists := r.conns[connID]
			r.mu.RUnlock()

			if exists {
				// Close signal from backend
				log.Printf("Connection %d closed by backend", connID)
				existing.Close()
				r.mu.Lock()
				delete(r.conns, connID)
				r.mu.Unlock()
				atomic.AddInt32(&r.stats.activeConns, -1)
			} else {
				// New connection request - connect to real usbmuxd
				r.handleNewConnection(connID, client)
			}
			continue
		}

		// Data for existing connection
		payload := make([]byte, length)
		if _, err := io.ReadFull(client, payload); err != nil {
			log.Printf("Backend payload read error: %v", err)
			break
		}

		r.mu.RLock()
		conn, ok := r.conns[connID]
		r.mu.RUnlock()

		if ok {
			if _, err := conn.Write(payload); err != nil {
				log.Printf("Connection %d write error: %v", connID, err)
				conn.Close()
				r.mu.Lock()
				delete(r.conns, connID)
				r.mu.Unlock()
				atomic.AddInt32(&r.stats.activeConns, -1)
				r.sendClose(client, connID)
			} else {
				atomic.AddUint64(&r.stats.bytesRelayed, uint64(len(payload)))
			}
		}
	}

	log.Printf("Backend disconnected")
	r.clientMu.Lock()
	if r.client == client {
		r.client = nil
	}
	r.clientMu.Unlock()
}

func (r *relay) handleNewConnection(connID uint32, client net.Conn) {
	conn, err := net.Dial("unix", *usbmuxdPath)
	if err != nil {
		log.Printf("Failed to connect to usbmuxd for conn %d: %v", connID, err)
		r.sendClose(client, connID)
		return
	}

	r.mu.Lock()
	r.conns[connID] = conn
	r.mu.Unlock()

	atomic.AddUint64(&r.stats.totalConns, 1)
	atomic.AddInt32(&r.stats.activeConns, 1)
	log.Printf("Connection %d: connected to usbmuxd", connID)

	// Read from usbmuxd, forward to backend
	go func() {
		defer func() {
			conn.Close()
			r.mu.Lock()
			delete(r.conns, connID)
			r.mu.Unlock()
			atomic.AddInt32(&r.stats.activeConns, -1)
			r.sendClose(client, connID)
			log.Printf("Connection %d: closed", connID)
		}()

		buf := make([]byte, 65536)
		for {
			n, err := conn.Read(buf)
			if err != nil {
				if err != io.EOF {
					log.Printf("Connection %d usbmuxd read error: %v", connID, err)
				}
				return
			}

			atomic.AddUint64(&r.stats.bytesRelayed, uint64(n))

			// Send to backend
			header := make([]byte, 8)
			binary.BigEndian.PutUint32(header[0:4], connID)
			binary.BigEndian.PutUint32(header[4:8], uint32(n))

			r.clientMu.Lock()
			if r.client != nil {
				r.client.Write(header)
				r.client.Write(buf[:n])
			}
			r.clientMu.Unlock()
		}
	}()
}

func (r *relay) sendClose(client net.Conn, connID uint32) {
	header := make([]byte, 8)
	binary.BigEndian.PutUint32(header[0:4], connID)
	binary.BigEndian.PutUint32(header[4:8], 0)

	r.clientMu.Lock()
	if r.client == client {
		client.Write(header)
	}
	r.clientMu.Unlock()
}

func main() {
	flag.Parse()
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// Setup log file if specified
	if *logFile != "" {
		// Truncate log file on start (wipe previous logs)
		f, err := os.OpenFile(*logFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			log.Fatalf("Failed to open log file %s: %v", *logFile, err)
		}
		log.SetOutput(f)
		// Don't close the file - it needs to stay open for logging
	}

	// Check for existing instance
	if existingPid, running := checkExistingProcess(); running {
		log.Fatalf("Another instance is already running (PID %d). Stop it first or remove %s", existingPid, getPidFilePath())
	}

	// Verify usbmuxd socket exists
	if _, err := os.Stat(*usbmuxdPath); os.IsNotExist(err) {
		log.Fatalf("usbmuxd socket not found at %s - is usbmuxd running?", *usbmuxdPath)
	}

	// Test connection to usbmuxd
	testConn, err := net.Dial("unix", *usbmuxdPath)
	if err != nil {
		log.Fatalf("Cannot connect to usbmuxd at %s: %v", *usbmuxdPath, err)
	}
	testConn.Close()
	log.Printf("Verified usbmuxd is accessible at %s", *usbmuxdPath)

	// Write PID file
	if err := writePidFile(); err != nil {
		log.Fatalf("Failed to write PID file: %v", err)
	}
	log.Printf("PID file written to %s", getPidFilePath())

	relay := newRelay()

	// Start TCP listener
	addr := fmt.Sprintf("%s:%d", *listenAddr, *listenPort)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		removePidFile()
		log.Fatalf("Failed to listen on %s: %v", addr, err)
	}
	defer listener.Close()

	log.Printf("USB Muxd relay listening on %s", addr)
	log.Printf("Backend should connect to host.docker.internal:%d", *listenPort)

	// Handle shutdown gracefully
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		log.Printf("Shutting down...")
		removePidFile()
		listener.Close()
		os.Exit(0)
	}()

	// Accept backend connections
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Accept error: %v", err)
			continue
		}

		go relay.handleClient(conn)
	}
}
