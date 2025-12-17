// backend/main.go
// USB Muxd backend - runs in Docker VM, manages iOS 17.4+ tunnels
package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"time"
)

var (
	socketPath = flag.String("socket", "/run/guest-services/backend.sock", "Unix socket for API")
)

func getHostAddr() string {
	if addr := os.Getenv("USBMUXD_SOCKET_ADDRESS"); addr != "" {
		return addr
	}
	return "host.docker.internal:27015"
}

// Tunnel manager instance (for iOS 17.4+ devices)
var tunnelManager *TunnelManager

// checkHostConnection does a quick TCP dial to see if host relay is reachable
func checkHostConnection() bool {
	conn, err := net.DialTimeout("tcp", getHostAddr(), 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// startTunnelInfoAPI starts an HTTP server compatible with go-ios tunnel info API
func startTunnelInfoAPI() {
	mux := http.NewServeMux()

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

	// Start tunnel manager for iOS 17.4+ devices
	tunnelManager = NewTunnelManager()
	tunnelManager.Start(context.Background())
	log.Printf("Tunnel manager started for iOS 17.4+ devices")

	// Start go-ios compatible tunnel info API on port 60105
	go startTunnelInfoAPI()

	// Start API server
	go startAPI()

	log.Printf("Backend ready. Containers should use USBMUXD_SOCKET_ADDRESS=host.docker.internal:27015")

	// Keep running for tunnel management and API
	select {}
}
