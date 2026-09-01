package service

import (
	"context"
	"errors"
	"net/http"

	"github.com/dash-xd/gospace/router"
)

// Dispatch serves exactly one request through the named router without
// changing the worker's active/default router.
func (s *Service) Dispatch(name string, rw http.ResponseWriter, req *http.Request) error {
	return s.worker.ServeRouter(name, rw, req)
}

// LoadAndDispatchWASM is the cold-instance convenience path for a
// self-contained dispatch carrying router bytes. If name is already
// registered, the existing immutable handler is reused. Otherwise module is
// compiled and registered under name, then only this request is dispatched to
// it. The active/default router is never changed.
//
// The returned boolean reports whether this call registered the router.
func (s *Service) LoadAndDispatchWASM(ctx context.Context, name string, module []byte, rw http.ResponseWriter, req *http.Request) (digest string, loaded bool, err error) {
	if _, ok := s.worker.Handler(name); ok {
		if err := s.worker.ServeRouter(name, rw, req); err != nil {
			return "", false, err
		}
		return "", false, nil
	}

	digest, err = s.RegisterWASM(ctx, name, module)
	if err != nil {
		// A concurrent request may have won registration for this immutable
		// name after our initial lookup. Reuse it rather than treating the
		// benign race as a failed dispatch.
		if errors.Is(err, router.ErrRouterExists) {
			if err := s.worker.ServeRouter(name, rw, req); err != nil {
				return "", false, err
			}
			return "", false, nil
		}
		return "", false, err
	}
	if err := s.worker.ServeRouter(name, rw, req); err != nil {
		return digest, true, err
	}
	return digest, true, nil
}
