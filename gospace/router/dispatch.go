package router

import "net/http"

// Handler returns an immutable registered handler by name without changing the
// worker's globally active/default router. This is the primitive used by
// request-scoped dispatchers that need to select a router for one request.
func (w *Worker) Handler(name string) (http.Handler, bool) {
	w.mu.RLock()
	handler, ok := w.handlers[name]
	w.mu.RUnlock()
	return handler, ok
}

// ServeRouter dispatches one request to a named registered router without
// mutating the worker's active/default router. Concurrent requests may safely
// select different immutable routers.
func (w *Worker) ServeRouter(name string, rw http.ResponseWriter, req *http.Request) error {
	handler, ok := w.Handler(name)
	if !ok {
		return ErrUnknownRouter
	}
	handler.ServeHTTP(rw, req)
	return nil
}
