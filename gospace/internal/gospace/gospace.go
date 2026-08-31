package gospace

import (
	"context"
	"net/http"

	"github.com/dash-xd/gospace/service"
	"github.com/dash-xd/gospace/internal/token"
	"github.com/dash-xd/gospace/internal/util"
)

type Fn func(http.ResponseWriter, *http.Request)

var worker = newWorker()

func newWorker() *service.Service {
	s := service.New()
	_ = s.RegisterFunc("util", util.Main)
	_ = s.RegisterFunc("token", token.Main)
	return s
}

func Main(w http.ResponseWriter, r *http.Request) {
	worker.ServeHTTP(w, r)
}

func RegisterFunc(name string, fn Fn) error {
	return worker.RegisterFunc(name, fn)
}

func Register(name string, handler http.Handler) error {
	return worker.Register(name, handler)
}

func Activate(name string) error {
	return worker.Activate(name)
}

func LoadWASM(ctx context.Context, name string, module []byte, activate bool) (string, error) {
	return worker.LoadWASM(ctx, name, module, activate)
}

func Active() string {
	return worker.Active()
}
