package main

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"

	"github.com/dash-xd/gospace/registry"
)

func main() {
	port, socketPath, initial := parseArgs()
	if initial != "" {
		if err := registry.Activate(initial); err != nil {
			fmt.Fprintf(os.Stderr, "activate router %q: %v\n", initial, err)
			os.Exit(1)
		}
	}

	handler := http.HandlerFunc(registry.ServeHTTP)
	if socketPath != "" {
		if err := serveUnix(socketPath, handler); err != nil {
			fmt.Fprintf(os.Stderr, "server error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	addr := fmt.Sprintf(":%d", port)
	fmt.Printf("gospace worker is listening on http://localhost%s\n", addr)
	if err := http.ListenAndServe(addr, handler); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}

func serveUnix(path string, handler http.Handler) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create socket directory: %w", err)
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove stale socket: %w", err)
	}

	listener, err := net.Listen("unix", path)
	if err != nil {
		return fmt.Errorf("listen on unix socket: %w", err)
	}
	defer listener.Close()
	defer os.Remove(path)

	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("set socket permissions: %w", err)
	}
	fmt.Printf("gospace worker is listening on unix://%s\n", path)
	return http.Serve(listener, handler)
}

func parseArgs() (int, string, string) {
	portPtr := flag.Int("port", 6060, "Port for the server to listen on")
	socketPtr := flag.String("unix-socket", "", "Unix-domain socket to listen on instead of TCP")
	initialPtr := flag.String("router", "", "Optional pre-registered router to activate at startup")
	flag.Parse()

	if *socketPtr == "" && (*portPtr <= 0 || *portPtr > 65535) {
		fmt.Fprintln(os.Stderr, "port must be between 1 and 65535")
		os.Exit(2)
	}
	return *portPtr, *socketPtr, *initialPtr
}
