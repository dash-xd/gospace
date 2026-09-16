package wasmguest

import (
	"bytes"
	"net/http"
	"unsafe"

	"github.com/dash-xd/gospace/wasmhttp"
)

var (
	input  []byte
	output []byte
)

type responseWriter struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func newResponseWriter() *responseWriter { return &responseWriter{header: make(http.Header)} }
func (w *responseWriter) Header() http.Header { return w.header }
func (w *responseWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
}
func (w *responseWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.body.Write(p)
}

// Alloc reserves request storage in the guest and returns its linear-memory
// offset. The exported WASM ABI is intentionally integer-based: host-visible
// addresses are offsets into WASM linear memory, never Go pointers.
func Alloc(size uint32) uint32 {
	input = make([]byte, size)
	if len(input) == 0 {
		return 0
	}
	return uint32(uintptr(unsafe.Pointer(&input[0])))
}

// Handle executes an ordinary net/http handler and returns the linear-memory
// offset of the encoded response. ResponseLen supplies the matching length.
func Handle(handler http.Handler) uint32 {
	if handler == nil {
		return encodeError(http.StatusInternalServerError, "nil router")
	}
	in, err := wasmhttp.DecodeRequest(input)
	if err != nil {
		return encodeError(http.StatusBadRequest, "invalid host request")
	}
	req, err := http.NewRequest(in.Method, in.URL, bytes.NewReader(in.Body))
	if err != nil {
		return encodeError(http.StatusBadRequest, "invalid request URL")
	}
	req.Host = in.Host
	req.Header = in.Header
	rw := newResponseWriter()
	handler.ServeHTTP(rw, req)
	if rw.status == 0 {
		rw.status = http.StatusOK
	}
	return encode(wasmhttp.Response{Status: rw.status, Header: rw.header, Body: rw.body.Bytes()})
}

func ResponseLen() uint32 { return uint32(len(output)) }

func encodeError(status int, message string) uint32 {
	return encode(wasmhttp.Response{
		Status: status,
		Header: http.Header{"Content-Type": {"text/plain; charset=utf-8"}},
		Body:   []byte(message + "\n"),
	})
}

func encode(response wasmhttp.Response) uint32 {
	output = wasmhttp.EncodeResponse(response)
	if len(output) == 0 {
		return 0
	}
	return uint32(uintptr(unsafe.Pointer(&output[0])))
}
