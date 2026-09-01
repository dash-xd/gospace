package service

import (
	"errors"
	"fmt"
)

var ErrRouterDigestMismatch = errors.New("router digest mismatch")

// WASMDigest returns the SHA-256 digest recorded for a WASM-backed router.
// Native routers have no WASM digest and return ok=false.
func (s *Service) WASMDigest(name string) (digest string, ok bool) {
	s.wasmMu.RLock()
	digest, ok = s.wasmDigests[name]
	s.wasmMu.RUnlock()
	return digest, ok
}

// requireWASMDigest verifies that an already-registered router is the exact
// immutable WASM artifact requested by a content-addressed dispatch.
func (s *Service) requireWASMDigest(name, expected string) error {
	if expected == "" {
		return nil
	}
	actual, ok := s.WASMDigest(name)
	if !ok || actual != expected {
		return fmt.Errorf("%w: router %q has %q, want %q", ErrRouterDigestMismatch, name, actual, expected)
	}
	return nil
}
