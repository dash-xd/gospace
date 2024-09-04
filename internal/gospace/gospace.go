package gospace

import (
	"net/http"

	"github.com/dash-xd/gospace/internal/util"
)

type Fn func(http.ResponseWriter, *http.Request)

var fns = make(map[string]Fn)

func RegisterFunc(pkg string, fn Fn) {
	fns[pkg] = fn
}

func init() {
	RegisterFunc("util", util.Main)
}

func GetRouter(key string) Fn {
	return fns[key]
}
