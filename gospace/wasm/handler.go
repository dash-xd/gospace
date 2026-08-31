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
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

const defaultMaxBody = 8 << 20

type Options struct {
	MaxRequestBody int64
	MaxResponseBody uint32
}

type Handler struct {
	mu sync.Mutex
	runtime wazero.Runtime
	module api.Module
	alloc api.Function
	handle api.Function
	maxRequestBody int64
	maxResponseBody uint32
}

func NewHandler(ctx context.Context, wasmBytes []byte, options Options) (*Handler, error) {
	if len(wasmBytes) == 0 { return nil, errors.New("empty WASM module") }
	if options.MaxRequestBody <= 0 { options.MaxRequestBody = defaultMaxBody }
	if options.MaxResponseBody == 0 { options.MaxResponseBody = defaultMaxBody }

	runtime := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfig().WithCloseOnContextDone(true).WithMemoryLimitPages(256))
	ok := false
	defer func() { if !ok { _ = runtime.Close(ctx) } }()

	if _, err := wasi_snapshot_preview1.Instantiate(ctx, runtime); err != nil { return nil, fmt.Errorf("instantiate WASI: %w", err) }
	compiled, err := runtime.CompileModule(ctx, wasmBytes)
	if err != nil { return nil, fmt.Errorf("compile WASM router: %w", err) }
	module, err := runtime.InstantiateModule(ctx, compiled, wazero.NewModuleConfig().WithStartFunctions("_initialize"))
	if err != nil { return nil, fmt.Errorf("instantiate WASM router: %w", err) }

	alloc := module.ExportedFunction("gospace_alloc")
	handle := module.ExportedFunction("gospace_handle")
	if alloc == nil || handle == nil { _ = module.Close(ctx); return nil, errors.New("WASM router must export gospace_alloc and gospace_handle") }

	ok = true
	return &Handler{runtime: runtime, module: module, alloc: alloc, handle: handle, maxRequestBody: options.MaxRequestBody, maxResponseBody: options.MaxResponseBody}, nil
}

func (h *Handler) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.runtime.Close(context.Background())
}

func (h *Handler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	body, err := readBody(req.Body, h.maxRequestBody)
	if err != nil { http.Error(rw, err.Error(), http.StatusRequestEntityTooLarge); return }
	payload, err := json.Marshal(wasmhttp.Request{Method: req.Method, URL: req.URL.RequestURI(), Host: req.Host, Header: wasmhttp.CloneHeader(req.Header), Body: body})
	if err != nil { http.Error(rw, "failed to encode request", http.StatusInternalServerError); return }

	h.mu.Lock()
	defer h.mu.Unlock()
	ctx := req.Context()
	allocResult, err := h.alloc.Call(ctx, uint64(uint32(len(payload))))
	if err != nil || len(allocResult) != 1 { http.Error(rw, "WASM request allocation failed", http.StatusBadGateway); return }
	ptr := uint32(allocResult[0])
	if len(payload) != 0 && !h.module.Memory().Write(ptr, payload) { http.Error(rw, "WASM request memory write failed", http.StatusBadGateway); return }

	handleResult, err := h.handle.Call(ctx)
	if err != nil || len(handleResult) != 1 { http.Error(rw, "WASM router execution failed", http.StatusBadGateway); return }
	packed := handleResult[0]
	responsePtr, responseLen := uint32(packed>>32), uint32(packed)
	if responseLen > h.maxResponseBody { http.Error(rw, "WASM response exceeds configured limit", http.StatusBadGateway); return }
	encoded, ok := h.module.Memory().Read(responsePtr, responseLen)
	if !ok { http.Error(rw, "WASM response memory read failed", http.StatusBadGateway); return }
	encoded = append([]byte(nil), encoded...)

	var response wasmhttp.Response
	if err := json.Unmarshal(encoded, &response); err != nil { http.Error(rw, "invalid WASM response", http.StatusBadGateway); return }
	for key, values := range response.Header { for _, value := range values { rw.Header().Add(key, value) } }
	status := response.Status
	if status < 100 || status > 999 { status = http.StatusOK }
	rw.WriteHeader(status)
	_, _ = rw.Write(response.Body)
}

func readBody(body io.ReadCloser, limit int64) ([]byte, error) {
	if body == nil { return nil, nil }
	defer body.Close()
	data, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err != nil { return nil, fmt.Errorf("read request body: %w", err) }
	if int64(len(data)) > limit { return nil, fmt.Errorf("request body exceeds %d bytes", limit) }
	return data, nil
}
