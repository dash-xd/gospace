package main

import (
	"fmt"
	"net/http"
	"os"
	"strconv"

	"github.com/dash-xd/gospace/internal/gospace"
)

func main() {
	port := getPortFromArgs()
	key := getKeyFromArgs()
	addr := fmt.Sprintf(":%d", port)

	fmt.Printf("Go Server is listening on http://localhost%s\n", addr)

	err := http.ListenAndServe(addr, http.HandlerFunc(gospace.GetRouter(key)))
	if err != nil {
		panic(err)
	}
}

func getPortFromArgs() int {
	defaultPort := 6060
	if len(os.Args) > 1 {
		port, err := strconv.Atoi(os.Args[1])
		if err != nil {
			fmt.Println("invalid port provided. using default port:", defaultPort)
			return defaultPort
		}
		return port
	}
	return defaultPort
}

func getKeyFromArgs() string {
	if len(os.Args) > 2 {
		return os.Args[2]
	}
	fmt.Println("no key provided. using default key: 'default'")
	return "default"
}
