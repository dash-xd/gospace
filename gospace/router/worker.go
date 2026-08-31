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
)

type activeHandler struct {
	name    string
	handler http.Handler
}

// Worker is a stable http.Handler whose active implementation can be changed
// without replacing the handler registered with the HTTP server or functions
// runtime.
type Worker struct {
	mu       sync.RWMutex
	handlers map[string]http.Handler
	active   atomic.Pointer[activeHandler]
}

func NewWorker() *Worker {
	return &Worker{handlers: make(map[string]http.Handler)}
}

// Register publishes an immutable named handler. Router names are intentionally
// not overwritten: versioned registrations avoid closing or mutating a handler
// that may still be serving an in-flight request.
func (w *Worker) Register(name string, handler http.Handler) error {
	if name == "" || handler == nil {
		return errors.New("router name and handler are required")
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	if _, exists := w.handlers[name]; exists {
		return ErrRouterExists
	}
	w.handlers[name] = handler
	return nil
}

func (w *Worker) RegisterFunc(name string, fn func(http.ResponseWriter, *http.Request)) error {
	if fn == nil {
		return errors.New("router function is required")
	}
	return w.Register(name, http.HandlerFunc(fn))
}

// Activate atomically redirects subsequent requests to a registered handler.
// Requests already executing continue on the handler they loaded.
func (w *Worker) Activate(name string) error {
	w.mu.RLock()
	handler, ok := w.handlers[name]
	w.mu.RUnlock()
	if !ok {
		return ErrUnknownRouter
	}

	w.active.Store(&activeHandler{name: name, handler: handler})
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

func (w *Worker) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	active := w.active.Load()
	if active == nil {
		http.Error(rw, "no router is active", http.StatusServiceUnavailable)
		return
	}
	active.handler.ServeHTTP(rw, req)
}
