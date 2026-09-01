package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"

	"github.com/dash-xd/gospace/server"
)

func main() {
	wasmPath := flag.String("wasm", "router.wasm", "WASM router to pre-register at startup")
	flag.Parse()

	module, err := os.ReadFile(*wasmPath)
	if err != nil {
		log.Fatal(err)
	}

	app := server.New(nil)
	if _, err := app.RegisterWASMRoutes(
		context.Background(),
		"users-v1",
		module,
		[]string{"GET /users/{id}"},
	); err != nil {
		log.Fatal(err)
	}

	log.Fatal(http.ListenAndServe(":8080", app))
}
