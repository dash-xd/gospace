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

func Activate(name string) error {
	return Default.Activate(name)
}

func LoadWASM(ctx context.Context, name string, module []byte, activate bool) (string, error) {
	return Default.LoadWASM(ctx, name, module, activate)
}

func Active() string {
	return Default.Active()
}

func ServeHTTP(w http.ResponseWriter, r *http.Request) {
	Default.ServeHTTP(w, r)
}
