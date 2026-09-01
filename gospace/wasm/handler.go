package wasm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/dash-xd/gospace/wasmhttp"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

const (
	defaultMaxBody          = 8 << 20
	defaultMemoryLimitPages = 1024 // 64 MiB per guest memory.
)

type Options struct {
	MaxRequestBody  int64
	MaxResponseBody uint32
}

type EngineOptions struct {
	MemoryLimitPages uint32
}

// Engine owns the process-level wazero runtime and shared WASI host. Individual
// routers only own compiled modules; requests still instantiate isolated guest
// modules from those compiled artifacts.
type Engine struct {
	runtime wazero.Runtime
}

func NewEngine(ctx context.Context, options EngineOptions) (*Engine, error) {
	if options.MemoryLimitPages == 0 {
		options.MemoryLimitPages = defaultMemoryLimitPages
	}
	runtimeConfig := wazero.NewRuntimeConfig().
		WithCloseOnContextDone(true).
		WithMemoryLimitPages(options.MemoryLimitPages)
	runtime := wazero.NewRuntimeWithConfig(ctx, runtimeConfig)
	if _, err := wasi_snapshot_preview1.Instantiate(ctx, runtime); err != nil {
		_ = runtime.Close(ctx)
		return nil, fmt.Errorf("instantiate WASI: %w", err)
	}
	return &Engine{runtime: runtime}, nil
}

func (e *Engine) Close(ctx context.Context) error {
	if e == nil || e.runtime == nil {
		return nil
	}
	return e.runtime.Close(ctx)
}

func (e *Engine) Compile(ctx context.Context, wasmBytes []byte, options Options) (*Handler, error) {
	if len(wasmBytes) == 0 {
		return nil, errors.New("empty WASM module")
	}
	if options.MaxRequestBody <= 0 {
		options.MaxRequestBody = defaultMaxBody
	}
	if options.MaxResponseBody == 0 {
		options.MaxResponseBody = defaultMaxBody
	}
	compiled, err := e.runtime.CompileModule(ctx, wasmBytes)
	if err != nil {
		return nil, fmt.Errorf("compile WASM router: %w", err)
	}
	return &Handler{
		engine:          e,
		compiled:        compiled,
		maxRequestBody:  options.MaxRequestBody,
		maxResponseBody: options.MaxResponseBody,
	}, nil
}

// Handler owns one compiled router. It leases itself while requests execute so
// cache eviction can close the compiled artifact after all in-flight requests
// have completed.
type Handler struct {
	engine          *Engine
	compiled        wazero.CompiledModule
	maxRequestBody  int64
	maxResponseBody uint32
	ownsEngine      bool

	mu      sync.Mutex
	cond    *sync.Cond
	active  int
	closing bool
}

// NewHandler remains as a standalone convenience API. Services that host many
// routers should create one Engine and call Engine.Compile instead.
func NewHandler(ctx context.Context, wasmBytes []byte, options Options) (*Handler, error) {
	engine, err := NewEngine(ctx, EngineOptions{})
	if err != nil {
		return nil, err
	}
	h, err := engine.Compile(ctx, wasmBytes, options)
	if err != nil {
		_ = engine.Close(ctx)
		return nil, err
	}
	h.ownsEngine = true
	return h, nil
}

func (h *Handler) begin() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.cond == nil {
		h.cond = sync.NewCond(&h.mu)
	}
	if h.closing {
		return false
	}
	h.active++
	return true
}

func (h *Handler) end() {
	h.mu.Lock()
	h.active--
	if h.closing && h.active == 0 && h.cond != nil {
		h.cond.Broadcast()
	}
	h.mu.Unlock()
}

func (h *Handler) Close() error {
	h.mu.Lock()
	if h.cond == nil {
		h.cond = sync.NewCond(&h.mu)
	}
	h.closing = true
	for h.active != 0 {
		h.cond.Wait()
	}
	h.mu.Unlock()

	err := h.compiled.Close(context.Background())
	if h.ownsEngine {
		if closeErr := h.engine.Close(context.Background()); err == nil {
			err = closeErr
		}
	}
	return err
}

func (h *Handler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	if !h.begin() {
		http.Error(rw, "WASM router is being evicted", http.StatusServiceUnavailable)
		return
	}
	defer h.end()

	body, err := readBody(req.Body, h.maxRequestBody)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusRequestEntityTooLarge)
		return
	}
	payload, err := json.Marshal(wasmhttp.Request{
		Method: req.Method,
		URL:    req.URL.RequestURI(),
		Host:   req.Host,
		Header: wasmhttp.CloneHeader(req.Header),
		Body:   body,
	})
	if err != nil {
		http.Error(rw, "failed to encode request", http.StatusInternalServerError)
		return
	}

	ctx := req.Context()
	module, err := h.engine.runtime.InstantiateModule(ctx, h.compiled, wazero.NewModuleConfig().WithName("").WithStartFunctions("_initialize"))
	if err != nil {
		http.Error(rw, "WASM router initialization failed", http.StatusBadGateway)
		return
	}
	defer module.Close(context.Background())

	alloc := module.ExportedFunction("gospace_alloc")
	handle := module.ExportedFunction("gospace_handle")
	responseLenFn := module.ExportedFunction("gospace_response_len")
	if alloc == nil || handle == nil || responseLenFn == nil {
		http.Error(rw, "WASM router has an incompatible gospace ABI", http.StatusBadGateway)
		return
	}

	allocResult, err := alloc.Call(ctx, uint64(uint32(len(payload))))
	if err != nil || len(allocResult) != 1 {
		http.Error(rw, "WASM request allocation failed", http.StatusBadGateway)
		return
	}
	requestPtr := uint32(allocResult[0])
	if len(payload) != 0 && !module.Memory().Write(requestPtr, payload) {
		http.Error(rw, "WASM request memory write failed", http.StatusBadGateway)
		return
	}

	handleResult, err := handle.Call(ctx)
	if err != nil || len(handleResult) != 1 {
		http.Error(rw, "WASM router execution failed", http.StatusBadGateway)
		return
	}
	responsePtr := uint32(handleResult[0])

	lenResult, err := responseLenFn.Call(ctx)
	if err != nil || len(lenResult) != 1 {
		http.Error(rw, "WASM response length failed", http.StatusBadGateway)
		return
	}
	responseLen := uint32(lenResult[0])
	if responseLen > h.maxResponseBody {
		http.Error(rw, "WASM response exceeds configured limit", http.StatusBadGateway)
		return
	}
	encoded, ok := module.Memory().Read(responsePtr, responseLen)
	if !ok {
		http.Error(rw, "WASM response memory read failed", http.StatusBadGateway)
		return
	}
	encoded = append([]byte(nil), encoded...)

	var response wasmhttp.Response
	if err := json.Unmarshal(encoded, &response); err != nil {
		http.Error(rw, "invalid WASM response", http.StatusBadGateway)
		return
	}
	for key, values := range response.Header {
		for _, value := range values {
			rw.Header().Add(key, value)
		}
	}
	status := response.Status
	if status < 100 || status > 999 {
		status = http.StatusOK
	}
	rw.WriteHeader(status)
	_, _ = rw.Write(response.Body)
}

func readBody(body io.ReadCloser, limit int64) ([]byte, error) {
	if body == nil {
		return nil, nil
	}
	data, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read request body: %w", err)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("request body exceeds %d bytes", limit)
	}
	return data, nil
}
