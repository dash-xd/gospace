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

func RegisterFunc(name string, fn func(http.ResponseWriter, *http.Request)) error {
	return Default.RegisterFunc(name, fn)
}

// RegisterWASM pre-registers bundled WASM module bytes without activating the
// router. It is intended to be usable from init alongside native Register calls.
func RegisterWASM(name string, module []byte) (string, error) {
	return Default.RegisterWASM(context.Background(), name, module)
}

func RegisterWASMContext(ctx context.Context, name string, module []byte) (string, error) {
	return Default.RegisterWASM(ctx, name, module)
}

func Activate(name string) error {
	return Default.Activate(name)
}

// Dispatch serves one request through name without changing the active/default
// router. Concurrent requests can independently select different routers.
func Dispatch(name string, w http.ResponseWriter, r *http.Request) error {
	return Default.Dispatch(name, w, r)
}

// DispatchDigest serves one request through a cached WASM router only when the
// registered artifact matches expectedDigest.
func DispatchDigest(name, expectedDigest string, w http.ResponseWriter, r *http.Request) error {
	return Default.DispatchDigest(name, expectedDigest, w, r)
}

// LoadAndDispatchWASM is the self-contained cold-instance path: verify/load an
// inline WASM router when needed and dispatch only the supplied request through
// it, without changing the active/default router.
func LoadAndDispatchWASM(ctx context.Context, name, expectedDigest string, module []byte, w http.ResponseWriter, r *http.Request) (digest string, loaded bool, err error) {
	return Default.LoadAndDispatchWASM(ctx, name, expectedDigest, module, w, r)
}

// LoadWASM registers module bytes that arrive at runtime and can immediately
// activate the resulting router. Internally this uses the same registration
// path as RegisterWASM.
func LoadWASM(ctx context.Context, name string, module []byte, activate bool) (string, error) {
	return Default.LoadWASM(ctx, name, module, activate)
}

func Active() string {
	return Default.Active()
}

func ServeHTTP(w http.ResponseWriter, r *http.Request) {
	Default.ServeHTTP(w, r)
}
