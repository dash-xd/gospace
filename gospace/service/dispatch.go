package service

import (
	"context"
	"net/http"
)

// Dispatch serves exactly one request through the named router without
// changing the worker's active/default router.
func (s *Service) Dispatch(name string, rw http.ResponseWriter, req *http.Request) error {
	s.touchWASM(name)
	return s.worker.ServeRouter(name, rw, req)
}

// DispatchDigest serves one request through a previously registered WASM
// router only when its recorded SHA-256 matches expectedDigest.
func (s *Service) DispatchDigest(name, expectedDigest string, rw http.ResponseWriter, req *http.Request) error {
	if err := s.requireWASMDigest(name, expectedDigest); err != nil {
		return err
	}
	s.touchWASM(name)
	return s.worker.ServeRouter(name, rw, req)
}

// dispatchThenEvict guarantees that the request which caused a cold router to
// be admitted gets a handler lease before capacity enforcement may retire that
// router. This is deliberately small so the bounded-cache sequencing can be
// tested without needing a WASM compiler fixture.
func (s *Service) dispatchThenEvict(name string, rw http.ResponseWriter, req *http.Request) error {
	defer s.evictWASMIfNeeded()
	return s.worker.ServeRouter(name, rw, req)
}

// LoadAndDispatchWASM preserves the original direct-only API. Call
// LoadAndDispatchWASMRoutes when the loaded router should participate in
// subsequent catchall route discovery.
func (s *Service) LoadAndDispatchWASM(ctx context.Context, name, expectedDigest string, module []byte, rw http.ResponseWriter, req *http.Request) (digest string, loaded bool, err error) {
	return s.LoadAndDispatchWASMRoutes(ctx, name, expectedDigest, module, nil, rw, req)
}

// LoadAndDispatchWASMRoutes verifies and singleflights a cold WASM load keyed
// by immutable name+digest, records route metadata, then dispatches only this
// request. Concurrent callers reuse the same compiled artifact.
func (s *Service) LoadAndDispatchWASMRoutes(ctx context.Context, name, expectedDigest string, module []byte, patterns []string, rw http.ResponseWriter, req *http.Request) (digest string, loaded bool, err error) {
	digest, loaded, err = s.ensureWASM(ctx, name, expectedDigest, module, patterns)
	if err != nil {
		return digest, loaded, err
	}

	// A cold router must be allowed to serve the request that caused it to be
	// compiled before bounded-cache eviction runs. This matters when every older
	// entry is pinned (for example the active router) and the new router would
	// otherwise be the only immediate eviction candidate.
	if err := s.dispatchThenEvict(name, rw, req); err != nil {
		return digest, loaded, err
	}
	return digest, loaded, nil
}
