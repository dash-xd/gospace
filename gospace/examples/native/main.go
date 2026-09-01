package main

import (
	"log"
	"net/http"

	"github.com/dash-xd/gospace/server"
)

func nativeRouter() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /hello", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("hello from native Go\n"))
	})
	return mux
}

func main() {
	app := server.New(nativeRouter())
	log.Fatal(http.ListenAndServe(":8080", app))
}
