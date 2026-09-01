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
	RouterRoutesHeader = "X-Gospace-Routes"
	requestPartType    = "application/vnd.gospace.request+json"
	routesPartType     = "application/vnd.gospace.routes+json"
	maxRequestPart     = 16 << 20
	maxRoutesPart      = 256 << 10
)

type routeManifest struct {
	Patterns []string `json:"patterns"`
}

// serveHinted is the optimized direct path. Cached routers execute immediately.
// A cold request may carry WASM bytes, immutable route metadata, and the actual
// application request in one multipart envelope.
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

	module, patterns, req, err := s.readHintEnvelope(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if len(patterns) == 0 {
		patterns = splitRoutePatterns(r.Header.Get(RouterRoutesHeader))
	}

	if cached {
		if err := s.requireRoutes(name, patterns); err != nil {
			writeDispatchError(w, err)
			return
		}
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

	_, _, err = s.LoadAndDispatchWASMRoutes(r.Context(), name, digest, module, patterns, w, req)
	writeDispatchError(w, err)
}

func (s *Service) readHintEnvelope(r *http.Request) ([]byte, []string, *http.Request, error) {
	mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/related" || params["boundary"] == "" {
		return nil, nil, nil, errors.New("Content-Type must be multipart/related with a boundary")
	}

	reader := multipart.NewReader(r.Body, params["boundary"])
	var module []byte
	var patterns []string
	var wire *wasmhttp.Request

	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, nil, nil, fmt.Errorf("read multipart dispatch: %w", err)
		}

		partType, _, _ := mime.ParseMediaType(part.Header.Get("Content-Type"))
		switch partType {
		case "application/wasm":
			if module != nil {
				return nil, nil, nil, errors.New("dispatch contains more than one application/wasm part")
			}
			module, err = io.ReadAll(io.LimitReader(part, s.maxWASM+1))
			if err != nil {
				return nil, nil, nil, fmt.Errorf("read WASM part: %w", err)
			}
			if int64(len(module)) > s.maxWASM {
				return nil, nil, nil, fmt.Errorf("WASM module exceeds %d bytes", s.maxWASM)
			}
		case routesPartType:
			if patterns != nil {
				return nil, nil, nil, errors.New("dispatch contains more than one routes part")
			}
			payload, err := io.ReadAll(io.LimitReader(part, maxRoutesPart+1))
			if err != nil {
				return nil, nil, nil, fmt.Errorf("read routes part: %w", err)
			}
			if len(payload) > maxRoutesPart {
				return nil, nil, nil, errors.New("routes part is too large")
			}
			var manifest routeManifest
			if err := json.Unmarshal(payload, &manifest); err != nil {
				return nil, nil, nil, fmt.Errorf("decode routes part: %w", err)
			}
			patterns = manifest.Patterns
		case requestPartType:
			if wire != nil {
				return nil, nil, nil, errors.New("dispatch contains more than one request part")
			}
			payload, err := io.ReadAll(io.LimitReader(part, maxRequestPart+1))
			if err != nil {
				return nil, nil, nil, fmt.Errorf("read request part: %w", err)
			}
			if len(payload) > maxRequestPart {
				return nil, nil, nil, errors.New("request part is too large")
			}
			var value wasmhttp.Request
			if err := json.Unmarshal(payload, &value); err != nil {
				return nil, nil, nil, fmt.Errorf("decode request part: %w", err)
			}
			wire = &value
		}
	}

	if wire == nil {
		return nil, nil, nil, fmt.Errorf("%s part is required", requestPartType)
	}
	if wire.Method == "" {
		return nil, nil, nil, errors.New("request method is required")
	}
	if wire.URL == "" {
		return nil, nil, nil, errors.New("request URL is required")
	}

	req, err := http.NewRequestWithContext(r.Context(), wire.Method, wire.URL, bytes.NewReader(wire.Body))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("reconstruct request: %w", err)
	}
	req.Host = wire.Host
	for key, values := range wire.Header {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	stripHintHeaders(req.Header)
	return module, patterns, req, nil
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
	header.Del(RouterRoutesHeader)
	header.Del(ControlTokenHeader)
}

func writeDispatchError(w http.ResponseWriter, err error) {
	if err == nil {
		return
	}
	switch {
	case errors.Is(err, router.ErrUnknownRouter):
		http.Error(w, "unknown router", http.StatusNotFound)
	case errors.Is(err, ErrRouterDigestMismatch), errors.Is(err, ErrRouterRoutesMismatch):
		http.Error(w, err.Error(), http.StatusConflict)
	default:
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
}
