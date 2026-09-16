package service

import (
	"context"
	"crypto/subtle"
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
	ControlPrefix         = "/_gospace/"
	ControlTokenHeader    = "X-Gospace-Control-Token"
	defaultMaxWASM        = 32 << 20
	defaultMaxWASMRouters = 64
)

type Options struct {
	ControlToken   string
	MaxWASMBytes   int64
	MaxWASMRouters int
	// Native is the deployment-composed Go application. Gospace does not
	// inspect or register its internal routes. When no gospace-managed router
	// owns a request, the request is delegated to Native exactly once.
	// Nil means no native application and defaults to http.NotFoundHandler().
	Native http.Handler
}

type Service struct {
	worker       *router.Worker
	control      *http.ServeMux
	controlToken string
	maxWASM      int64
	native       http.Handler

	wasmEngine     *wasmrouter.Engine
	wasmEngineErr  error
	maxWASMRouters int
	wasm           wasmRegistry
	loads          loadGroup
}

func New() *Service {
	return NewWithOptions(Options{ControlToken: os.Getenv("GOSPACE_CONTROL_TOKEN")})
}

func NewWithNative(native http.Handler) *Service {
	return NewWithOptions(Options{
		ControlToken: os.Getenv("GOSPACE_CONTROL_TOKEN"),
		Native:       native,
	})
}

func NewWithOptions(options Options) *Service {
	if options.MaxWASMBytes <= 0 {
		options.MaxWASMBytes = defaultMaxWASM
	}
	if options.MaxWASMRouters <= 0 {
		options.MaxWASMRouters = defaultMaxWASMRouters
	}
	if options.Native == nil {
		options.Native = http.NotFoundHandler()
	}
	engine, engineErr := wasmrouter.NewEngine(context.Background(), wasmrouter.EngineOptions{})
	s := &Service{
		worker:          router.NewWorker(),
		controlToken:    options.ControlToken,
		maxWASM:         options.MaxWASMBytes,
		native:          options.Native,
		wasmEngine:      engine,
		wasmEngineErr:   engineErr,
		maxWASMRouters:  options.MaxWASMRouters,
		wasm:            newWASMRegistry(),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /_gospace/routers", s.listRouters)
	mux.HandleFunc("GET /_gospace/resolve", s.resolveRoute)
	mux.HandleFunc("POST /_gospace/activate/{name}", s.activateRouter)
	mux.HandleFunc("POST /_gospace/wasm/{name}", s.loadWASM)
	s.control = mux
	return s
}

func (s *Service) Register(name string, handler http.Handler) error {
	return s.worker.Register(name, handler)
}

func (s *Service) RegisterRoutes(name string, handler http.Handler, patterns []string) error {
	return s.worker.RegisterRoutes(name, handler, patterns)
}

func (s *Service) RegisterFunc(name string, fn func(http.ResponseWriter, *http.Request)) error {
	return s.worker.RegisterFunc(name, fn)
}

func (s *Service) RegisterFuncRoutes(name string, fn func(http.ResponseWriter, *http.Request), patterns []string) error {
	return s.worker.RegisterFuncRoutes(name, fn, patterns)
}

func (s *Service) RegisterWASM(ctx context.Context, name string, module []byte) (string, error) {
	return s.RegisterWASMRoutes(ctx, name, module, nil)
}

func (s *Service) RegisterWASMRoutes(ctx context.Context, name string, module []byte, patterns []string) (string, error) {
	digest, err := s.registerWASM(ctx, name, module, patterns)
	if err != nil {
		return "", err
	}
	s.evictWASMIfNeeded()
	return digest, nil
}

func (s *Service) Activate(name string) error {
	return s.worker.Activate(name)
}

func (s *Service) Active() string { return s.worker.Active() }

func (s *Service) LoadWASM(ctx context.Context, name string, module []byte, activate bool) (string, error) {
	return s.LoadWASMRoutes(ctx, name, module, nil, activate)
}

func (s *Service) LoadWASMRoutes(ctx context.Context, name string, module []byte, patterns []string, activate bool) (string, error) {
	digest, err := s.registerWASM(ctx, name, module, patterns)
	if err != nil {
		return "", err
	}
	if activate {
		if err := s.worker.Activate(name); err != nil {
			s.evictWASMIfNeeded()
			return "", err
		}
	}
	s.evictWASMIfNeeded()
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

	if r.Header.Get(RouterHeader) != "" {
		s.serveHinted(w, r)
		return
	}
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

func (s *Service) resolveRoute(w http.ResponseWriter, r *http.Request) {
	method := r.URL.Query().Get("method")
	path := r.URL.Query().Get("path")
	if method == "" || path == "" || path[0] != '/' {
		http.Error(w, "method and absolute path are required", http.StatusBadRequest)
		return
	}
	probe, err := http.NewRequest(method, path, nil)
	if err != nil {
		http.Error(w, "invalid route probe", http.StatusBadRequest)
		return
	}
	name, ok := s.worker.Match(probe)
	if !ok {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"router": name})
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

	patterns := splitRoutePatterns(r.Header.Get(RouterRoutesHeader))
	activate := r.URL.Query().Get("activate") == "true"
	digest, err := s.LoadWASMRoutes(r.Context(), name, module, patterns, activate)
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
		"routes":    patterns,
		"sizeBytes": len(module),
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
