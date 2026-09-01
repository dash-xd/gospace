package router

import (
	"errors"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
)

var (
	ErrUnknownRouter = errors.New("unknown router")
	ErrRouterExists  = errors.New("router already registered")
	ErrRouterActive  = errors.New("active router cannot be removed")
)

type activeHandler struct {
	name string
}

type handlerEntry struct {
	handler http.Handler
	mu      sync.Mutex
	cond    *sync.Cond
	refs    int
	retired bool
}

func newHandlerEntry(handler http.Handler) *handlerEntry {
	entry := &handlerEntry{handler: handler}
	entry.cond = sync.NewCond(&entry.mu)
	return entry
}

func (e *handlerEntry) acquire() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.retired {
		return false
	}
	e.refs++
	return true
}

func (e *handlerEntry) release() {
	e.mu.Lock()
	e.refs--
	if e.retired && e.refs == 0 {
		e.cond.Broadcast()
	}
	e.mu.Unlock()
}

func (e *handlerEntry) retire() {
	e.mu.Lock()
	e.retired = true
	for e.refs != 0 {
		e.cond.Wait()
	}
	e.mu.Unlock()
}

// Worker is a stable http.Handler whose immutable router registrations can be
// selected by name or by non-executing route metadata.
type Worker struct {
	mu       sync.RWMutex
	handlers map[string]*handlerEntry
	routes   map[string][]string
	active   atomic.Pointer[activeHandler]
	table    atomic.Pointer[routeTable]
}

func NewWorker() *Worker {
	w := &Worker{
		handlers: make(map[string]*handlerEntry),
		routes:   make(map[string][]string),
	}
	w.table.Store(emptyRouteTable())
	return w
}

// Register publishes an immutable named handler without catchall route
// metadata. It can still be selected explicitly or activated as the default.
func (w *Worker) Register(name string, handler http.Handler) error {
	return w.RegisterRoutes(name, handler, nil)
}

// RegisterRoutes publishes an immutable named handler and the net/http
// ServeMux patterns it owns. Patterns are used only for matching; handlers are
// never executed speculatively during route discovery.
func (w *Worker) RegisterRoutes(name string, handler http.Handler, patterns []string) error {
	if name == "" || handler == nil {
		return errors.New("router name and handler are required")
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	if _, exists := w.handlers[name]; exists {
		return ErrRouterExists
	}

	nextRoutes := cloneRoutes(w.routes)
	if len(patterns) != 0 {
		nextRoutes[name] = append([]string(nil), patterns...)
	}
	table, err := buildRouteTable(nextRoutes)
	if err != nil {
		return err
	}

	w.handlers[name] = newHandlerEntry(handler)
	if len(patterns) != 0 {
		w.routes[name] = append([]string(nil), patterns...)
	}
	w.table.Store(table)
	return nil
}

func (w *Worker) RegisterFunc(name string, fn func(http.ResponseWriter, *http.Request)) error {
	return w.RegisterFuncRoutes(name, fn, nil)
}

func (w *Worker) RegisterFuncRoutes(name string, fn func(http.ResponseWriter, *http.Request), patterns []string) error {
	if fn == nil {
		return errors.New("router function is required")
	}
	return w.RegisterRoutes(name, http.HandlerFunc(fn), patterns)
}

// Activate atomically redirects subsequent default requests to a registered
// router. Request-scoped dispatch remains independent of this selection.
func (w *Worker) Activate(name string) error {
	w.mu.RLock()
	_, ok := w.handlers[name]
	w.mu.RUnlock()
	if !ok {
		return ErrUnknownRouter
	}
	w.active.Store(&activeHandler{name: name})
	return nil
}

func (w *Worker) RegisterAndActivate(name string, handler http.Handler) error {
	if err := w.Register(name, handler); err != nil {
		return err
	}
	return w.Activate(name)
}

func (w *Worker) Active() string {
	active := w.active.Load()
	if active == nil {
		return ""
	}
	return active.name
}

func (w *Worker) Names() []string {
	w.mu.RLock()
	names := make([]string, 0, len(w.handlers))
	for name := range w.handlers {
		names = append(names, name)
	}
	w.mu.RUnlock()
	sort.Strings(names)
	return names
}

func (w *Worker) Routes(name string) []string {
	w.mu.RLock()
	patterns := append([]string(nil), w.routes[name]...)
	w.mu.RUnlock()
	return patterns
}

// Match resolves route ownership using immutable metadata only. No application
// handler is called until after a single owner has been selected.
func (w *Worker) Match(req *http.Request) (string, bool) {
	table := w.table.Load()
	if table == nil {
		return "", false
	}
	return table.match(req)
}

// Remove retires a non-active router. It disappears from named and route-index
// lookups before this method waits for any requests that already leased it.
// The returned handler is therefore safe for the caller to close.
func (w *Worker) Remove(name string) (http.Handler, error) {
	if w.Active() == name {
		return nil, ErrRouterActive
	}

	w.mu.Lock()
	entry, ok := w.handlers[name]
	if !ok {
		w.mu.Unlock()
		return nil, ErrUnknownRouter
	}

	nextRoutes := cloneRoutes(w.routes)
	delete(nextRoutes, name)
	table, err := buildRouteTable(nextRoutes)
	if err != nil {
		w.mu.Unlock()
		return nil, err
	}
	delete(w.handlers, name)
	delete(w.routes, name)
	w.table.Store(table)
	w.mu.Unlock()

	entry.retire()
	return entry.handler, nil
}

func (w *Worker) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	active := w.active.Load()
	if active == nil {
		http.Error(rw, "no router is active", http.StatusServiceUnavailable)
		return
	}
	if err := w.ServeRouter(active.name, rw, req); err != nil {
		http.Error(rw, "active router is unavailable", http.StatusServiceUnavailable)
	}
}
