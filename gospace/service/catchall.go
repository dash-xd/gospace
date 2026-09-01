package service

import "net/http"

// serveCatchall first resolves gospace-managed native/WASM registrations from
// immutable route metadata. If none owns the request, it delegates exactly once
// to the deployment-composed native handler. No candidate handler is executed
// as part of discovery and the request body is never buffered or replayed.
func (s *Service) serveCatchall(w http.ResponseWriter, r *http.Request) {
	name, ok := s.worker.Match(r)
	if !ok {
		s.native.ServeHTTP(w, cloneWithoutHintHeaders(r))
		return
	}
	if err := s.Dispatch(name, w, cloneWithoutHintHeaders(r)); err != nil {
		writeDispatchError(w, err)
	}
}
