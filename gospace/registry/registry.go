package registry

import (
	"context"
	"net/http"

	"github.com/dash-xd/gospace/service"
)

var Default = service.New()

func Register(name string, handler http.Handler) error {
	return Default.Register(name, handler)
}

func RegisterRoutes(name string, handler http.Handler, patterns []string) error {
	return Default.RegisterRoutes(name, handler, patterns)
}

func RegisterFunc(name string, fn func(http.ResponseWriter, *http.Request)) error {
	return Default.RegisterFunc(name, fn)
}

func RegisterFuncRoutes(name string, fn func(http.ResponseWriter, *http.Request), patterns []string) error {
	return Default.RegisterFuncRoutes(name, fn, patterns)
}

func RegisterWASM(name string, module []byte) (string, error) {
	return Default.RegisterWASM(context.Background(), name, module)
}

func RegisterWASMRoutes(name string, module []byte, patterns []string) (string, error) {
	return Default.RegisterWASMRoutes(context.Background(), name, module, patterns)
}

func RegisterWASMContext(ctx context.Context, name string, module []byte) (string, error) {
	return Default.RegisterWASM(ctx, name, module)
}

func RegisterWASMRoutesContext(ctx context.Context, name string, module []byte, patterns []string) (string, error) {
	return Default.RegisterWASMRoutes(ctx, name, module, patterns)
}

func Activate(name string) error {
	return Default.Activate(name)
}

func Dispatch(name string, w http.ResponseWriter, r *http.Request) error {
	return Default.Dispatch(name, w, r)
}

func DispatchDigest(name, expectedDigest string, w http.ResponseWriter, r *http.Request) error {
	return Default.DispatchDigest(name, expectedDigest, w, r)
}

func LoadAndDispatchWASM(ctx context.Context, name, expectedDigest string, module []byte, w http.ResponseWriter, r *http.Request) (digest string, loaded bool, err error) {
	return Default.LoadAndDispatchWASM(ctx, name, expectedDigest, module, w, r)
}

func LoadAndDispatchWASMRoutes(ctx context.Context, name, expectedDigest string, module []byte, patterns []string, w http.ResponseWriter, r *http.Request) (digest string, loaded bool, err error) {
	return Default.LoadAndDispatchWASMRoutes(ctx, name, expectedDigest, module, patterns, w, r)
}

func LoadWASM(ctx context.Context, name string, module []byte, activate bool) (string, error) {
	return Default.LoadWASM(ctx, name, module, activate)
}

func LoadWASMRoutes(ctx context.Context, name string, module []byte, patterns []string, activate bool) (string, error) {
	return Default.LoadWASMRoutes(ctx, name, module, patterns, activate)
}

func Active() string {
	return Default.Active()
}

func ServeHTTP(w http.ResponseWriter, r *http.Request) {
	Default.ServeHTTP(w, r)
}
