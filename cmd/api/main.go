package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"strconv"

	"github.com/dash-xd/gospace/internal/gospace"
)

func main() {
	port, key := parseArgs()
	addr := fmt.Sprintf(":%d", port)

	fmt.Printf("Go Server is listening on http://localhost%s\n", addr)

	err := http.ListenAndServe(addr, http.HandlerFunc(gospace.GetRouter(key)))
	if err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}

func parseArgs() (int, string) {
	defaultPort := 6060
	defaultKey := "default"

	portPtr := flag.Int("port", defaultPort, "Port for the server to listen on")
	keyPtr := flag.String("key", defaultKey, "Key that determines which router to run from gospace")

	flag.Parse()

	if *portPtr <= 0 || *portPtr > 65535 {
		fmt.Fprintf(os.Stderr, "Invalid port: %d. Using default port: %d\n", *portPtr, defaultPort)
		*portPtr = defaultPort
	}

	return *portPtr, *keyPtr
}
