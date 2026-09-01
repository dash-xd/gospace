package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"

	"github.com/dash-xd/gospace/router"
)

// Dispatch serves exactly one request through the named router without
// changing the worker's active/default router.
func (s *Service) Dispatch(name string, rw http.ResponseWriter, req *http.Request) error {
	return s.worker.ServeRouter(name, rw, req)
}

// DispatchDigest serves one request through a previously registered WASM
// router only when its recorded SHA-256 matches expectedDigest. It never
// changes the active/default router.
func (s *Service) DispatchDigest(name, expectedDigest string, rw http.ResponseWriter, req *http.Request) error {
	if err := s.requireWASMDigest(name, expectedDigest); err != nil {
		return err
	}
	return s.worker.ServeRouter(name, rw, req)
}

// LoadAndDispatchWASM is the cold-instance convenience path for a
// self-contained dispatch carrying router bytes. If name is already
// registered, expectedDigest must match the cached immutable WASM artifact and
// the existing handler is reused. Otherwise module is verified, compiled and
// registered under name, then only this request is dispatched to it. The
// active/default router is never changed.
//
// The returned boolean reports whether this call registered the router.
func (s *Service) LoadAndDispatchWASM(ctx context.Context, name, expectedDigest string, module []byte, rw http.ResponseWriter, req *http.Request) (digest string, loaded bool, err error) {
	if _, ok := s.worker.Handler(name); ok {
		if err := s.requireWASMDigest(name, expectedDigest); err != nil {
			return "", false, err
		}
		digest, _ = s.WASMDigest(name)
		if err := s.worker.ServeRouter(name, rw, req); err != nil {
			return digest, false, err
		}
		return digest, false, nil
	}

	if expectedDigest != "" {
		sum := sha256.Sum256(module)
		actual := hex.EncodeToString(sum[:])
		if actual != expectedDigest {
			return actual, false, ErrRouterDigestMismatch
		}
	}

	digest, err = s.RegisterWASM(ctx, name, module)
	if err != nil {
		// A concurrent request may have won registration for this immutable
		// name after our initial lookup. Reuse it only when its digest matches.
		if errors.Is(err, router.ErrRouterExists) {
			if err := s.requireWASMDigest(name, expectedDigest); err != nil {
				return "", false, err
			}
			digest, _ = s.WASMDigest(name)
			if err := s.worker.ServeRouter(name, rw, req); err != nil {
				return digest, false, err
			}
			return digest, false, nil
		}
		return "", false, err
	}
	if err := s.worker.ServeRouter(name, rw, req); err != nil {
		return digest, true, err
	}
	return digest, true, nil
}
