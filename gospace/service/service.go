package service

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/dash-xd/gospace/router"
	wasmrouter "github.com/dash-xd/gospace/wasm"
)

const (
	ControlPrefix      = "/_gospace/"
	ControlTokenHeader = "X-Gospace-Control-Token"
	defaultMaxWASM     = 32 << 20
)

type Options struct {
	ControlToken string
	MaxWASMBytes int64
}

type Service struct {
	worker       *router.Worker
	control      *http.ServeMux
	controlToken string
	maxWASM      int64
}

func New() *Service {
	return NewWithOptions(Options{ControlToken: os.Getenv("GOSPACE_CONTROL_TOKEN")})
}

func NewWithOptions(options Options) *Service {
	if options.MaxWASMBytes <= 0 {
		options.MaxWASMBytes = defaultMaxWASM
	}

	s := &Service{
		worker:       router.NewWorker(),
		controlToken: options.ControlToken,
		maxWASM:      options.MaxWASMBytes,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /_gospace/routers", s.listRouters)
	mux.HandleFunc("POST /_gospace/activate/{name}", s.activateRouter)
	mux.HandleFunc("POST /_gospace/wasm/{name}", s.loadWASM)
	s.control = mux
	return s
}

func (s *Service) Register(name string, handler http.Handler) error {
	return s.worker.Register(name, handler)
}

func (s *Service) RegisterFunc(name string, fn func(http.ResponseWriter, *http.Request)) error {
	return s.worker.RegisterFunc(name, fn)
}

func (s *Service) Activate(name string) error {
	return s.worker.Activate(name)
}

func (s *Service) Active() string { return s.worker.Active() }

// LoadWASM registers a WASM-backed http.Handler directly. This API does not
// depend on the HTTP control plane and is suitable for an authenticated control
// path that already has the module bytes.
func (s *Service) LoadWASM(ctx context.Context, name string, module []byte, activate bool) (string, error) {
	if name == "" {
		return "", errors.New("router name is required")
	}
	h, err := wasmrouter.NewHandler(ctx, module, wasmrouter.Options{})
	if err != nil {
		return "", err
	}
	if err := s.worker.Register(name, h); err != nil {
		_ = h.Close()
		return "", err
	}
	if activate {
		if err := s.worker.Activate(name); err != nil {
			return "", err
		}
	}
	sum := sha256.Sum256(module)
	return hex.EncodeToString(sum[:]), nil
}

func (s *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, ControlPrefix) {
		if !s.authorizeControl(r) {
			// Leave the privileged surface undiscoverable when runtime control is
			// disabled or the caller does not possess the separate control token.
			http.NotFound(w, r)
			return
		}
		s.control.ServeHTTP(w, r)
		return
	}
	s.worker.ServeHTTP(w, r)
}

func (s *Service) authorizeControl(r *http.Request) bool {
	if s.controlToken == "" {
		return false
	}
	provided := r.Header.Get(ControlTokenHeader)
	if len(provided) != len(s.controlToken) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(s.controlToken)) == 1
}

func (s *Service) listRouters(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"active":  s.worker.Active(),
		"routers": s.worker.Names(),
	})
}

func (s *Service) activateRouter(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.worker.Activate(name); err != nil {
		if errors.Is(err, router.ErrUnknownRouter) {
			http.Error(w, "unknown router", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"active": name})
}

func (s *Service) loadWASM(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	module, err := io.ReadAll(io.LimitReader(r.Body, s.maxWASM+1))
	if err != nil {
		http.Error(w, "failed to read WASM module", http.StatusBadRequest)
		return
	}
	if int64(len(module)) > s.maxWASM {
		http.Error(w, fmt.Sprintf("WASM module exceeds %d bytes", s.maxWASM), http.StatusRequestEntityTooLarge)
		return
	}

	activate := r.URL.Query().Get("activate") == "true"
	digest, err := s.LoadWASM(r.Context(), name, module, activate)
	if err != nil {
		if errors.Is(err, router.ErrRouterExists) {
			http.Error(w, "router name already registered; use an immutable/versioned name", http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"name":      name,
		"sha256":    digest,
		"active":    activate,
		"sizeBytes": len(module),
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
