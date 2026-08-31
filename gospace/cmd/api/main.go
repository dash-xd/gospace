package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"

	"github.com/dash-xd/gospace/internal/gospace"
)

func main() {
	port, initial := parseArgs()
	if initial != "" {
		if err := gospace.Activate(initial); err != nil {
			fmt.Fprintf(os.Stderr, "activate router %q: %v\n", initial, err)
			os.Exit(1)
		}
	}

	addr := fmt.Sprintf(":%d", port)
	fmt.Printf("gospace worker is listening on http://localhost%s\n", addr)
	if err := http.ListenAndServe(addr, http.HandlerFunc(gospace.Main)); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}

func parseArgs() (int, string) {
	portPtr := flag.Int("port", 6060, "Port for the server to listen on")
	initialPtr := flag.String("router", "", "Optional pre-registered router to activate at startup")
	flag.Parse()

	if *portPtr <= 0 || *portPtr > 65535 {
		fmt.Fprintln(os.Stderr, "port must be between 1 and 65535")
		os.Exit(2)
	}
	return *portPtr, *initialPtr
}
