package wasmguest

import (
	"bytes"
	"encoding/json"
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

func newResponseWriter() *responseWriter {
	return &responseWriter{header: make(http.Header)}
}

func (w *responseWriter) Header() http.Header {
	return w.header
}

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

// Alloc reserves guest memory for the host request payload. Router entry
// packages export a tiny go:wasmexport wrapper around this function.
func Alloc(size uint32) unsafe.Pointer {
	input = make([]byte, size)
	if len(input) == 0 {
		return nil
	}
	return unsafe.Pointer(&input[0])
}

// Handle adapts the current wire request to an ordinary net/http Handler,
// stores the encoded response in guest memory, and returns its address. The
// go:wasmexport wrapper lets the Go compiler perform the defined
// unsafe.Pointer -> WebAssembly i32 translation.
func Handle(handler http.Handler) unsafe.Pointer {
	if handler == nil {
		return encodeError(http.StatusInternalServerError, "nil router")
	}

	var in wasmhttp.Request
	if err := json.Unmarshal(input, &in); err != nil {
		return encodeError(http.StatusBadRequest, "invalid host request")
	}

	req, err := http.NewRequest(in.Method, in.URL, bytes.NewReader(in.Body))
	if err != nil {
		return encodeError(http.StatusBadRequest, "invalid request URL")
	}
	req.Host = in.Host
	for key, values := range in.Header {
		req.Header[key] = append([]string(nil), values...)
	}

	rw := newResponseWriter()
	handler.ServeHTTP(rw, req)
	if rw.status == 0 {
		rw.status = http.StatusOK
	}

	return encode(wasmhttp.Response{
		Status: rw.status,
		Header: wasmhttp.CloneHeader(rw.header),
		Body:   append([]byte(nil), rw.body.Bytes()...),
	})
}

func ResponseLen() uint32 {
	return uint32(len(output))
}

func encodeError(status int, message string) unsafe.Pointer {
	return encode(wasmhttp.Response{
		Status: status,
		Header: map[string][]string{"Content-Type": {"text/plain; charset=utf-8"}},
		Body:   []byte(message + "\n"),
	})
}

func encode(response wasmhttp.Response) unsafe.Pointer {
	encoded, err := json.Marshal(response)
	if err != nil {
		encoded = []byte(`{"status":500,"body":"d2FzbSByZXNwb25zZSBlbmNvZGluZyBmYWlsZWQK"}`)
	}
	output = encoded
	if len(output) == 0 {
		return nil
	}
	return unsafe.Pointer(&output[0])
}
