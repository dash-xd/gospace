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
	"sync"

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
	wasmMu       sync.RWMutex
	wasmDigests  map[string]string
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
		wasmDigests:  make(map[string]string),
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

func (s *Service) RegisterWASM(ctx context.Context, name string, module []byte) (string, error) {
	if name == "" {
		return "", errors.New("router name is required")
	}
	if int64(len(module)) > s.maxWASM {
		return "", fmt.Errorf("WASM module exceeds %d bytes", s.maxWASM)
	}

	sum := sha256.Sum256(module)
	digest := hex.EncodeToString(sum[:])

	h, err := wasmrouter.NewHandler(ctx, module, wasmrouter.Options{})
	if err != nil {
		return "", err
	}

	s.wasmMu.Lock()
	defer s.wasmMu.Unlock()
	if err := s.worker.Register(name, h); err != nil {
		_ = h.Close()
		return "", err
	}
	s.wasmDigests[name] = digest
	return digest, nil
}

func (s *Service) Activate(name string) error {
	return s.worker.Activate(name)
}

func (s *Service) Active() string { return s.worker.Active() }

func (s *Service) LoadWASM(ctx context.Context, name string, module []byte, activate bool) (string, error) {
	digest, err := s.RegisterWASM(ctx, name, module)
	if err != nil {
		return "", err
	}
	if activate {
		if err := s.worker.Activate(name); err != nil {
			return "", err
		}
	}
	return digest, nil
}

func (s *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, ControlPrefix) {
		if !s.authorizeControl(r) {
			http.NotFound(w, r)
			return
		}
		s.control.ServeHTTP(w, r)
		return
	}

	// A router header is an optional direct path. It can avoid catchall probing
	// and, on a cold miss, carry the artifact needed to load that router.
	if r.Header.Get(RouterHeader) != "" {
		s.serveHinted(w, r)
		return
	}

	// Without a hint, discover the route from routers already present in this
	// worker. The active router is tried first, then the remaining registrations.
	s.serveCatchall(w, r)
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
