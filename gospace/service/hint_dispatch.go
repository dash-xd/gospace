package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/dash-xd/gospace/router"
	"github.com/dash-xd/gospace/wasmhttp"
)

const (
	RouterHeader       = "X-Gospace-Router"
	RouterDigestHeader = "X-Gospace-Router-SHA256"
	requestPartType    = "application/vnd.gospace.request+json"
	maxRequestPart     = 16 << 20
)

// serveHinted dispatches one request through the named router. If the router is
// already cached, an ordinary request is enough. If it is missing, the same
// request may carry a multipart/related WASM artifact + original request and is
// loaded before that original request is dispatched.
func (s *Service) serveHinted(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.Header.Get(RouterHeader))
	if name == "" {
		http.Error(w, "gospace router hint is empty", http.StatusBadRequest)
		return
	}
	digest := strings.TrimSpace(r.Header.Get(RouterDigestHeader))

	_, cached := s.worker.Handler(name)
	mediaType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	selfContained := mediaType == "multipart/related"

	if cached && !selfContained {
		req := cloneWithoutHintHeaders(r)
		var err error
		if digest == "" {
			err = s.Dispatch(name, w, req)
		} else {
			err = s.DispatchDigest(name, digest, w, req)
		}
		writeDispatchError(w, err)
		return
	}

	if !selfContained {
		http.Error(w, "router is not cached; include multipart/related router artifact hint", http.StatusPreconditionRequired)
		return
	}

	module, req, err := s.readHintEnvelope(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if cached {
		if digest == "" {
			err = s.Dispatch(name, w, req)
		} else {
			err = s.DispatchDigest(name, digest, w, req)
		}
		writeDispatchError(w, err)
		return
	}

	if !s.authorizeControl(r) {
		http.NotFound(w, r)
		return
	}
	if digest == "" {
		http.Error(w, "X-Gospace-Router-SHA256 is required when loading WASM", http.StatusBadRequest)
		return
	}
	if len(module) == 0 {
		http.Error(w, "application/wasm part is required when router is not cached", http.StatusPreconditionRequired)
		return
	}

	_, _, err = s.LoadAndDispatchWASM(r.Context(), name, digest, module, w, req)
	writeDispatchError(w, err)
}

func (s *Service) readHintEnvelope(r *http.Request) ([]byte, *http.Request, error) {
	mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/related" || params["boundary"] == "" {
		return nil, nil, errors.New("Content-Type must be multipart/related with a boundary")
	}

	reader := multipart.NewReader(r.Body, params["boundary"])
	var module []byte
	var wire *wasmhttp.Request

	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, nil, fmt.Errorf("read multipart dispatch: %w", err)
		}

		partType, _, _ := mime.ParseMediaType(part.Header.Get("Content-Type"))
		switch partType {
		case "application/wasm":
			if module != nil {
				return nil, nil, errors.New("dispatch contains more than one application/wasm part")
			}
			module, err = io.ReadAll(io.LimitReader(part, s.maxWASM+1))
			if err != nil {
				return nil, nil, fmt.Errorf("read WASM part: %w", err)
			}
			if int64(len(module)) > s.maxWASM {
				return nil, nil, fmt.Errorf("WASM module exceeds %d bytes", s.maxWASM)
			}
		case requestPartType:
			if wire != nil {
				return nil, nil, errors.New("dispatch contains more than one request part")
			}
			var value wasmhttp.Request
			limited := io.LimitReader(part, maxRequestPart+1)
			payload, err := io.ReadAll(limited)
			if err != nil {
				return nil, nil, fmt.Errorf("read request part: %w", err)
			}
			if len(payload) > maxRequestPart {
				return nil, nil, errors.New("request part is too large")
			}
			if err := json.Unmarshal(payload, &value); err != nil {
				return nil, nil, fmt.Errorf("decode request part: %w", err)
			}
			wire = &value
		}
	}

	if wire == nil {
		return nil, nil, fmt.Errorf("%s part is required", requestPartType)
	}
	if wire.Method == "" {
		return nil, nil, errors.New("request method is required")
	}
	if wire.URL == "" {
		return nil, nil, errors.New("request URL is required")
	}

	req, err := http.NewRequestWithContext(r.Context(), wire.Method, wire.URL, bytes.NewReader(wire.Body))
	if err != nil {
		return nil, nil, fmt.Errorf("reconstruct request: %w", err)
	}
	req.Host = wire.Host
	for key, values := range wire.Header {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	stripHintHeaders(req.Header)
	return module, req, nil
}

func cloneWithoutHintHeaders(r *http.Request) *http.Request {
	clone := r.Clone(r.Context())
	clone.Header = r.Header.Clone()
	stripHintHeaders(clone.Header)
	return clone
}

func stripHintHeaders(header http.Header) {
	header.Del(RouterHeader)
	header.Del(RouterDigestHeader)
	header.Del(ControlTokenHeader)
}

func writeDispatchError(w http.ResponseWriter, err error) {
	if err == nil {
		return
	}
	switch {
	case errors.Is(err, router.ErrUnknownRouter):
		http.Error(w, "unknown router", http.StatusNotFound)
	case errors.Is(err, ErrRouterDigestMismatch):
		http.Error(w, err.Error(), http.StatusConflict)
	default:
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
}
