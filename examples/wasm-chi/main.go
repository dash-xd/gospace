package main

import (
	"net/http"
	"unsafe"

	"github.com/dash-xd/gospace/wasmguest"
	"github.com/go-chi/chi/v5"
)

var handler = newRouter()

func newRouter() http.Handler {
	r := chi.NewRouter()
	r.Get("/", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("hello from a hot-loaded Chi router\n"))
	})
	r.Get("/users/{id}", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("user: " + chi.URLParam(r, "id") + "\n"))
	})
	return r
}

//go:wasmexport gospace_alloc
func gospaceAlloc(size uint32) unsafe.Pointer {
	return wasmguest.Alloc(size)
}

//go:wasmexport gospace_handle
func gospaceHandle() uint64 {
	return wasmguest.Handle(handler)
}

func main() {}
