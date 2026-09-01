package router

import (
	"fmt"
	"net/http"
)

type routeOwner struct {
	name string
}

func (o routeOwner) ServeHTTP(http.ResponseWriter, *http.Request) {}

type routeTable struct {
	mux    *http.ServeMux
	owners map[string]string
}

func emptyRouteTable() *routeTable {
	return &routeTable{mux: http.NewServeMux(), owners: map[string]string{}}
}

func cloneRoutes(src map[string][]string) map[string][]string {
	dst := make(map[string][]string, len(src))
	for name, patterns := range src {
		dst[name] = append([]string(nil), patterns...)
	}
	return dst
}

func buildRouteTable(routes map[string][]string) (table *routeTable, err error) {
	mux := http.NewServeMux()
	owners := make(map[string]string)

	defer func() {
		if recovered := recover(); recovered != nil {
			table = nil
			err = fmt.Errorf("invalid or conflicting route metadata: %v", recovered)
		}
	}()

	for name, patterns := range routes {
		for _, pattern := range patterns {
			if pattern == "" {
				return nil, fmt.Errorf("router %q contains an empty route pattern", name)
			}
			mux.Handle(pattern, routeOwner{name: name})
			owners[pattern] = name
		}
	}
	return &routeTable{mux: mux, owners: owners}, nil
}

func (t *routeTable) match(req *http.Request) (string, bool) {
	_, pattern := t.mux.Handler(req)
	if pattern == "" {
		return "", false
	}
	name, ok := t.owners[pattern]
	return name, ok
}
