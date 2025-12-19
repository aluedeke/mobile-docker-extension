// backend/main.go
// USB Muxd backend - simple API server for Docker Desktop extension UI
// Also provides a Unix socket proxy to usbmuxd for containers
package main

import (
	"encoding/json"
	"flag"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"sync/atomic"
	"time"
)

var (
	socketPath       = flag.String("socket", "/run/guest-services/backend.sock", "Unix socket for API")
	usbmuxdSocket    = flag.String("usbmuxd-socket", "/run/guest-services/usbmuxd.sock", "Unix socket for usbmuxd proxy")
	activeConns      int64
)

func getHostAddr() string {
	if addr := os.Getenv("USBMUXD_SOCKET_ADDRESS"); addr != "" {
		return addr
	}
	return "host.docker.internal:27015"
}

// checkHostConnection does a quick TCP dial to see if host relay is reachable
func checkHostConnection() bool {
	conn, err := net.DialTimeout("tcp", getHostAddr(), 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// handleProxyConnection proxies data between a Unix socket client and the TCP host
func handleProxyConnection(clientConn net.Conn) {
	defer clientConn.Close()

	count := atomic.AddInt64(&activeConns, 1)
	defer atomic.AddInt64(&activeConns, -1)
	log.Printf("usbmuxd proxy: new connection (active: %d)", count)

	// Connect to host relay
	hostConn, err := net.DialTimeout("tcp", getHostAddr(), 5*time.Second)
	if err != nil {
		log.Printf("usbmuxd proxy: failed to connect to host: %v", err)
		return
	}
	defer hostConn.Close()

	// Bidirectional copy
	done := make(chan struct{}, 2)
	go func() {
		io.Copy(hostConn, clientConn)
		done <- struct{}{}
	}()
	go func() {
		io.Copy(clientConn, hostConn)
		done <- struct{}{}
	}()
	<-done
}

// startUsbmuxdProxy creates a Unix socket that proxies to the host's TCP relay
func startUsbmuxdProxy() {
	// Remove existing socket file
	os.RemoveAll(*usbmuxdSocket)

	listener, err := net.Listen("unix", *usbmuxdSocket)
	if err != nil {
		log.Printf("Warning: Failed to create usbmuxd proxy socket: %v", err)
		return
	}

	// Make socket world-accessible so containers can use it
	os.Chmod(*usbmuxdSocket, 0777)

	log.Printf("usbmuxd proxy listening on %s -> %s", *usbmuxdSocket, getHostAddr())

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("usbmuxd proxy: accept error: %v", err)
			continue
		}
		go handleProxyConnection(conn)
	}
}

func startAPI() {
	mux := http.NewServeMux()

	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		hostReachable := checkHostConnection()

		status := map[string]interface{}{
			"hostReachable": hostReachable,
			"hostAddress":   getHostAddr(),
		}
		json.NewEncoder(w).Encode(status)
	})

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if checkHostConnection() {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("OK"))
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("Host relay not reachable"))
		}
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

	log.Printf("Backend starting...")
	log.Printf("Host relay address: %s", getHostAddr())

	// Start usbmuxd proxy (Unix socket for containers)
	go startUsbmuxdProxy()

	// Start API server (Unix socket for extension UI)
	startAPI()
}
