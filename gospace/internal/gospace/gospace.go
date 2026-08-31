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
	mustRegister("util", util.Main)
	mustRegister("token", token.Main)
}

func mustRegister(name string, fn func(http.ResponseWriter, *http.Request)) {
	if err := registry.RegisterFunc(name, fn); err != nil {
		panic(err)
	}
}

func Main(w http.ResponseWriter, r *http.Request) {
	registry.ServeHTTP(w, r)
}

func RegisterFunc(name string, fn Fn) error {
	return registry.RegisterFunc(name, fn)
}

func Register(name string, handler http.Handler) error {
	return registry.Register(name, handler)
}

func Activate(name string) error {
	return registry.Activate(name)
}

func LoadWASM(ctx context.Context, name string, module []byte, activate bool) (string, error) {
	return registry.LoadWASM(ctx, name, module, activate)
}

func Active() string {
	return registry.Active()
}
