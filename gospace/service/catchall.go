package service

import "net/http"

// serveCatchall resolves one already-registered route using immutable route
// metadata. Discovery never executes candidate application handlers and never
// buffers or replays the request body.
func (s *Service) serveCatchall(w http.ResponseWriter, r *http.Request) {
	name, ok := s.worker.Match(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if err := s.Dispatch(name, w, cloneWithoutHintHeaders(r)); err != nil {
		writeDispatchError(w, err)
	}
}
