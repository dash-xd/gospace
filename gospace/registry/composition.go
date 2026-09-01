package registry

import (
	"fmt"
	"net/http"
)

// NativeRouter is the minimal contract for composing a statically linked Go
// router into gospace. Application packages own construction of Handler; gospace
// only needs a stable name plus optional route metadata for no-hint discovery.
//
// Routes use net/http ServeMux patterns, for example:
//
//	GET /users/{id}
//	POST /stream
//
// An empty Routes slice is valid and registers the handler as direct-only: it
// can still be selected with X-Gospace-Router, but catchall routing will not
// infer ownership by executing it.
type NativeRouter struct {
	Name    string
	Handler http.Handler
	Routes  []string
}

// Compose registers native routers into the process-global gospace registry.
// It intentionally does not import or know about application packages. A
// deployment-owned composition file supplies those imports and constructors.
func Compose(routers ...NativeRouter) error {
	for _, candidate := range routers {
		if candidate.Name == "" {
			return fmt.Errorf("native router name is required")
		}
		if candidate.Handler == nil {
			return fmt.Errorf("native router %q handler is required", candidate.Name)
		}
		if err := RegisterRoutes(candidate.Name, candidate.Handler, candidate.Routes); err != nil {
			return fmt.Errorf("register native router %q: %w", candidate.Name, err)
		}
	}
	return nil
}

// MustCompose is intended for deployment/package init functions. Invalid or
// conflicting composition should fail the process at startup rather than leave
// a partially useful worker running.
func MustCompose(routers ...NativeRouter) {
	if err := Compose(routers...); err != nil {
		panic(err)
	}
}
