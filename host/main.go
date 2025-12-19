// host/main.go
// USB Muxd relay - runs on macOS host, bridges TCP to real usbmuxd socket
// Also provides iOS 17.4+ tunnel management via go-ios
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/danielpaulus/go-ios/ios/tunnel"
	ps "github.com/mitchellh/go-ps"
)

var (
	usbmuxdPath = flag.String("usbmuxd", "/var/run/usbmuxd", "Path to real usbmuxd socket")
	listenPort  = flag.Int("port", 27015, "TCP port to listen on")
	listenAddr  = flag.String("addr", "127.0.0.1", "Address to listen on")
	dataDir     = flag.String("data-dir", "", "Directory for PID/log files (default: next to binary)")
	pidFile     = flag.String("pidfile", "", "Path to PID file (default: <data-dir>/mobile-relay.pid)")
	logFile     = flag.String("logfile", "", "Path to log file (default: stdout/stderr)")

	// Tunnel options
	enableTunnel   = flag.Bool("tunnel", true, "Enable iOS 17.4+ tunnel manager")
	tunnelPort     = flag.Int("tunnel-port", 60105, "Port for tunnel info API")
	pairRecordPath = flag.String("pair-records", "", "Path for pair records (default: data-dir)")
)

var stats struct {
	totalConns   uint64
	activeConns  int32
	bytesRelayed uint64
}

// getDataDir returns the directory for storing data files (PID, logs, pair records)
func getDataDir() string {
	if *dataDir != "" {
		return *dataDir
	}
	// Default to directory containing the binary
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}

func getPidFilePath() string {
	if *pidFile != "" {
		return *pidFile
	}
	return filepath.Join(getDataDir(), "mobile-relay.pid")
}

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

	// Use go-ps for cross-platform process checking
	proc, err := ps.FindProcess(pid)
	if err != nil || proc == nil {
		os.Remove(pidPath)
		return 0, false
	}

	// Check if it's actually mobile-relay
	procName := proc.Executable()
	if !strings.Contains(strings.ToLower(procName), "mobile-relay") {
		log.Printf("PID %d exists but is '%s', not mobile-relay - removing stale PID file", pid, procName)
		os.Remove(pidPath)
		return 0, false
	}

	return pid, true
}

func writePidFile() error {
	pidPath := getPidFilePath()
	dir := filepath.Dir(pidPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create PID file directory: %w", err)
	}
	pid := os.Getpid()
	return os.WriteFile(pidPath, []byte(strconv.Itoa(pid)), 0644)
}

func removePidFile() {
	os.Remove(getPidFilePath())
}

// handleConnection proxies data between TCP client and usbmuxd socket
func handleConnection(tcpConn net.Conn) {
	defer tcpConn.Close()

	atomic.AddUint64(&stats.totalConns, 1)
	atomic.AddInt32(&stats.activeConns, 1)
	defer atomic.AddInt32(&stats.activeConns, -1)

	// Connect to real usbmuxd
	unixConn, err := net.Dial("unix", *usbmuxdPath)
	if err != nil {
		log.Printf("Failed to connect to usbmuxd: %v", err)
		return
	}
	defer unixConn.Close()

	log.Printf("New connection from %s (active: %d)", tcpConn.RemoteAddr(), atomic.LoadInt32(&stats.activeConns))

	// Bidirectional copy
	done := make(chan struct{}, 2)

	// TCP -> Unix
	go func() {
		n, _ := io.Copy(unixConn, tcpConn)
		atomic.AddUint64(&stats.bytesRelayed, uint64(n))
		done <- struct{}{}
	}()

	// Unix -> TCP
	go func() {
		n, _ := io.Copy(tcpConn, unixConn)
		atomic.AddUint64(&stats.bytesRelayed, uint64(n))
		done <- struct{}{}
	}()

	// Wait for either direction to finish
	<-done
	log.Printf("Connection closed from %s (active: %d)", tcpConn.RemoteAddr(), atomic.LoadInt32(&stats.activeConns)-1)
}

// startTunnelManager starts the iOS 17.4+ tunnel manager
func startTunnelManager(ctx context.Context, recordsPath string, port int) (*tunnel.TunnelManager, error) {
	pm, err := tunnel.NewPairRecordManager(recordsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create pair record manager: %w", err)
	}

	// Use userspace TUN (no root/TUN device required)
	tm := tunnel.NewTunnelManager(pm, true)

	// Periodically check for devices and update tunnels
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				err := tm.UpdateTunnels(ctx)
				if err != nil {
					log.Printf("Tunnel update error: %v", err)
				}
			}
		}
	}()

	// Start HTTP API for tunnel info
	go func() {
		err := tunnel.ServeTunnelInfo(tm, port)
		if err != nil {
			log.Printf("Tunnel info server error: %v", err)
		}
	}()

	return tm, nil
}

func main() {
	flag.Parse()
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	if *logFile != "" {
		f, err := os.OpenFile(*logFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			log.Fatalf("Failed to open log file %s: %v", *logFile, err)
		}
		log.SetOutput(f)
	}

	if existingPid, running := checkExistingProcess(); running {
		log.Fatalf("Another instance is already running (PID %d). Stop it first or remove %s", existingPid, getPidFilePath())
	}

	// Check if usbmuxd socket exists
	usbmuxdAvailable := false
	if _, err := os.Stat(*usbmuxdPath); os.IsNotExist(err) {
		log.Printf("Warning: usbmuxd socket not found at %s", *usbmuxdPath)
		log.Printf("Socket relay will not be started - connect an iOS device to activate usbmuxd")
	} else if testConn, err := net.Dial("unix", *usbmuxdPath); err != nil {
		log.Printf("Warning: Cannot connect to usbmuxd at %s: %v", *usbmuxdPath, err)
		log.Printf("Socket relay will not be started until usbmuxd is accessible")
	} else {
		testConn.Close()
		log.Printf("Verified usbmuxd is accessible at %s", *usbmuxdPath)
		usbmuxdAvailable = true
	}

	if err := writePidFile(); err != nil {
		log.Fatalf("Failed to write PID file: %v", err)
	}
	log.Printf("PID file written to %s", getPidFilePath())

	// Start tunnel manager for iOS 17.4+ devices
	ctx, cancel := context.WithCancel(context.Background())
	var tm *tunnel.TunnelManager
	if *enableTunnel {
		recordsPath := *pairRecordPath
		if recordsPath == "" {
			recordsPath = getDataDir()
		}
		var err error
		tm, err = startTunnelManager(ctx, recordsPath, *tunnelPort)
		if err != nil {
			log.Printf("Warning: Failed to start tunnel manager: %v", err)
			log.Printf("iOS 17.4+ tunnel features will not be available")
		} else {
			log.Printf("iOS 17.4+ tunnel manager started on port %d", *tunnelPort)
			log.Printf("Containers should use GO_IOS_AGENT_HOST=host.docker.internal GO_IOS_AGENT_PORT=%d", *tunnelPort)
		}
	}

	// Only start socket relay if usbmuxd is available
	var listener net.Listener
	if usbmuxdAvailable {
		addr := fmt.Sprintf("%s:%d", *listenAddr, *listenPort)
		var err error
		listener, err = net.Listen("tcp", addr)
		if err != nil {
			removePidFile()
			log.Fatalf("Failed to listen on %s: %v", addr, err)
		}
		defer listener.Close()

		log.Printf("USB Muxd relay listening on %s", addr)
		log.Printf("Containers should use USBMUXD_SOCKET_ADDRESS=host.docker.internal:%d", *listenPort)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)

	go func() {
		<-sigCh
		log.Printf("Shutting down...")
		cancel()
		if tm != nil {
			tm.Close()
		}
		removePidFile()
		if listener != nil {
			listener.Close()
		}
		os.Exit(0)
	}()

	// Accept connections if socket relay is running
	if listener != nil {
		for {
			conn, err := listener.Accept()
			if err != nil {
				log.Printf("Accept error: %v", err)
				continue
			}
			go handleConnection(conn)
		}
	} else {
		// No socket relay, just wait for signal (tunnel manager may still be running)
		log.Printf("Running in tunnel-only mode (no socket relay)")
		select {}
	}
}
