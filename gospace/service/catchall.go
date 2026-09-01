package service

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
)

const maxCatchallBody = 16 << 20

// serveCatchall discovers which already-registered router owns this path.
// The active router is preferred, then remaining routers are tried in stable
// name order. A router that returns 404 is treated as a miss; any other status
// commits that router's response.
//
// Catchall discovery buffers the request and candidate response so a 404 can be
// discarded before trying the next router. Callers that already know the router
// should send X-Gospace-Router to take the direct path instead.
func (s *Service) serveCatchall(w http.ResponseWriter, r *http.Request) {
	names := s.worker.Names()
	if len(names) == 0 {
		http.NotFound(w, r)
		return
	}

	active := s.worker.Active()
	ordered := make([]string, 0, len(names))
	if active != "" {
		ordered = append(ordered, active)
	}
	for _, name := range names {
		if name != active {
			ordered = append(ordered, name)
		}
	}

	// If there is only one candidate, no probing is required and the handler can
	// retain the ordinary net/http response semantics.
	if len(ordered) == 1 {
		if err := s.Dispatch(ordered[0], w, cloneWithoutHintHeaders(r)); err != nil {
			writeDispatchError(w, err)
		}
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxCatchallBody+1))
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}
	if len(body) > maxCatchallBody {
		http.Error(w, "request body is too large for catchall discovery", http.StatusRequestEntityTooLarge)
		return
	}

	for _, name := range ordered {
		req := r.Clone(r.Context())
		req.Header = r.Header.Clone()
		stripHintHeaders(req.Header)
		req.Body = io.NopCloser(bytes.NewReader(body))
		req.ContentLength = int64(len(body))

		rr := httptest.NewRecorder()
		if err := s.Dispatch(name, rr, req); err != nil {
			continue
		}
		result := rr.Result()
		if result.StatusCode == http.StatusNotFound {
			_ = result.Body.Close()
			continue
		}
		copyRecordedResponse(w, result)
		return
	}

	http.NotFound(w, r)
}

func copyRecordedResponse(w http.ResponseWriter, response *http.Response) {
	defer response.Body.Close()
	for key, values := range response.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(response.StatusCode)
	_, _ = io.Copy(w, response.Body)
}
