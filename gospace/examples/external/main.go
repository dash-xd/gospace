//go:build external_example

package main

import (
	"log"
	"net/http"

	"github.com/dash-xd/gospace/server"
	newsrouter "github.com/xd-dash/news/router"
	stonksrouter "github.com/xd-dash/stonks/router"
)

func nativeRouter() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/news/", http.StripPrefix("/news", newsrouter.NewRouter()))
	mux.Handle("/stonks/", http.StripPrefix("/stonks", stonksrouter.NewRouter()))
	return mux
}

func main() {
	app := server.New(nativeRouter())
	log.Fatal(http.ListenAndServe(":8080", app))
}
