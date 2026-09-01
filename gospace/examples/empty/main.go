package main

import (
	"log"
	"net/http"

	"github.com/dash-xd/gospace/server"
)

func main() {
	// nil means no statically linked native application. The process can still
	// receive runtime-loaded WASM routers through gospace's normal control/hint
	// paths.
	app := server.New(nil)
	log.Fatal(http.ListenAndServe(":8080", app))
}
