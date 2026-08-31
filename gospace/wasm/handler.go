package wasm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/dash-xd/gospace/wasmhttp"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

const (
	defaultMaxBody          = 8 << 20
	defaultMemoryLimitPages = 1024 // 64 MiB per guest memory.
)

type Options struct {
	MaxRequestBody   int64
	MaxResponseBody  uint32
	MemoryLimitPages uint32
}

// Handler compiles a WASM router once and instantiates an isolated reactor for
// each HTTP request. This avoids sharing mutable guest state across concurrent
// Cloud Functions Gen 2 invocations and lets request cancellation terminate
// only that request's guest instance.
type Handler struct {
	runtime         wazero.Runtime
	compiled        wazero.CompiledModule
	maxRequestBody  int64
	maxResponseBody uint32
}

func NewHandler(ctx context.Context, wasmBytes []byte, options Options) (*Handler, error) {
	if len(wasmBytes) == 0 {
		return nil, errors.New("empty WASM module")
	}
	if options.MaxRequestBody <= 0 {
		options.MaxRequestBody = defaultMaxBody
	}
	if options.MaxResponseBody == 0 {
		options.MaxResponseBody = defaultMaxBody
	}
	if options.MemoryLimitPages == 0 {
		options.MemoryLimitPages = defaultMemoryLimitPages
	}

	runtimeConfig := wazero.NewRuntimeConfig().
		WithCloseOnContextDone(true).
		WithMemoryLimitPages(options.MemoryLimitPages)
	runtime := wazero.NewRuntimeWithConfig(ctx, runtimeConfig)
	ok := false
	defer func() {
		if !ok {
			_ = runtime.Close(ctx)
		}
	}()

	if _, err := wasi_snapshot_preview1.Instantiate(ctx, runtime); err != nil {
		return nil, fmt.Errorf("instantiate WASI: %w", err)
	}
	compiled, err := runtime.CompileModule(ctx, wasmBytes)
	if err != nil {
		return nil, fmt.Errorf("compile WASM router: %w", err)
	}

	ok = true
	return &Handler{
		runtime:         runtime,
		compiled:        compiled,
		maxRequestBody:  options.MaxRequestBody,
		maxResponseBody: options.MaxResponseBody,
	}, nil
}

func (h *Handler) Close() error {
	return h.runtime.Close(context.Background())
}

func (h *Handler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
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
	module, err := h.runtime.InstantiateModule(ctx, h.compiled, wazero.NewModuleConfig().WithName("").WithStartFunctions("_initialize"))
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
