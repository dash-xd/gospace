package gospace

import (
	"context"
	"net/http"

	"github.com/dash-xd/gospace/internal/token"
	"github.com/dash-xd/gospace/internal/util"
	"github.com/dash-xd/gospace/registry"
)

type Fn func(http.ResponseWriter, *http.Request)

func init() {
	mustRegisterRoutes("util", util.Main, []string{
		"POST /log",
		"/fs",
		"GET /headers",
		"GET /info",
	})
	mustRegisterRoutes("token", token.Main, []string{
		"/{$}",
		"/id",
		"/access",
	})
}

func mustRegisterRoutes(name string, fn func(http.ResponseWriter, *http.Request), patterns []string) {
	if err := registry.RegisterFuncRoutes(name, fn, patterns); err != nil {
		panic(err)
	}
}

func Main(w http.ResponseWriter, r *http.Request) {
	registry.ServeHTTP(w, r)
}

func RegisterFunc(name string, fn Fn) error {
	return registry.RegisterFunc(name, fn)
}

func RegisterFuncRoutes(name string, fn Fn, patterns []string) error {
	return registry.RegisterFuncRoutes(name, fn, patterns)
}

func Register(name string, handler http.Handler) error {
	return registry.Register(name, handler)
}

func RegisterRoutes(name string, handler http.Handler, patterns []string) error {
	return registry.RegisterRoutes(name, handler, patterns)
}

func RegisterWASM(name string, module []byte) (string, error) {
	return registry.RegisterWASM(name, module)
}

func RegisterWASMRoutes(name string, module []byte, patterns []string) (string, error) {
	return registry.RegisterWASMRoutes(name, module, patterns)
}

func RegisterWASMContext(ctx context.Context, name string, module []byte) (string, error) {
	return registry.RegisterWASMContext(ctx, name, module)
}

func RegisterWASMRoutesContext(ctx context.Context, name string, module []byte, patterns []string) (string, error) {
	return registry.RegisterWASMRoutesContext(ctx, name, module, patterns)
}

func Activate(name string) error {
	return registry.Activate(name)
}

func LoadWASM(ctx context.Context, name string, module []byte, activate bool) (string, error) {
	return registry.LoadWASM(ctx, name, module, activate)
}

func LoadWASMRoutes(ctx context.Context, name string, module []byte, patterns []string, activate bool) (string, error) {
	return registry.LoadWASMRoutes(ctx, name, module, patterns, activate)
}

func Active() string {
	return registry.Active()
}
