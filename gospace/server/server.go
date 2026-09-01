// Package server provides the small build-time composition surface for gospace.
// Applications supply an ordinary net/http Handler; nil means no statically
// linked native application. Dynamic and pre-registered WASM remain owned by
// the returned gospace service.
package server

import (
	"net/http"
	"os"

	"github.com/dash-xd/gospace/service"
)

// Options configures gospace runtime infrastructure without imposing any
// application routing abstraction on the native handler.
type Options struct {
	ControlToken   string
	MaxWASMBytes   int64
	MaxWASMRouters int
}

// New constructs a gospace service around a deployment-composed native
// handler. A nil handler is valid and means that gospace starts with no native
// application routes.
func New(native http.Handler) *service.Service {
	return NewWithOptions(native, Options{
		ControlToken: os.Getenv("GOSPACE_CONTROL_TOKEN"),
	})
}

// NewWithOptions is the configurable form of New.
func NewWithOptions(native http.Handler, options Options) *service.Service {
	return service.NewWithOptions(service.Options{
		ControlToken:   options.ControlToken,
		MaxWASMBytes:   options.MaxWASMBytes,
		MaxWASMRouters: options.MaxWASMRouters,
		Native:         native,
	})
}
