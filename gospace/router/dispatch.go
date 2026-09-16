package router

import "net/http"

// Handler returns an immutable registered handler by name without changing the
// worker's active/default router. Callers should use ServeRouter when executing
// it so cache eviction can wait for in-flight requests safely.
func (w *Worker) Handler(name string) (http.Handler, bool) {
	w.mu.RLock()
	entry, ok := w.handlers[name]
	w.mu.RUnlock()
	if !ok {
		return nil, false
	}
	return entry.handler, true
}

// ServeRouter dispatches one request to a named registered router without
// mutating the active/default router. A short lease keeps eviction from closing
// the handler while this request is running.
func (w *Worker) ServeRouter(name string, rw http.ResponseWriter, req *http.Request) error {
	w.mu.RLock()
	entry, ok := w.handlers[name]
	if !ok || !entry.acquire() {
		w.mu.RUnlock()
		return ErrUnknownRouter
	}
	w.mu.RUnlock()
	defer entry.release()

	entry.handler.ServeHTTP(rw, req)
	return nil
}
