package main

/*
#include <stdint.h>
#include <stdlib.h>

typedef struct {
    uint32_t status;
    uint8_t *headers;
    size_t headers_len;
    uint8_t *body;
    size_t body_len;
} gs_response;
*/
import "C"

import (
    "bytes"
    "fmt"
    "net/http"
    "net/url"
    "strings"
    "sync"
    "unsafe"

    "github.com/dash-xd/gospace/service"
)

var (
    runtimeOnce sync.Once
    runtimeService *service.Service
    deploymentRegister = func(*service.Service) {}
)

func runtime() *service.Service {
    runtimeOnce.Do(func() {
        s := service.New()
        deploymentRegister(s)
        runtimeService = s
    })
    return runtimeService
}

type responseWriter struct {
    header http.Header
    status int
    body bytes.Buffer
}

func (w *responseWriter) Header() http.Header { return w.header }
func (w *responseWriter) WriteHeader(status int) {
    if w.status == 0 { w.status = status }
}
func (w *responseWriter) Write(p []byte) (int, error) {
    if w.status == 0 { w.status = http.StatusOK }
    return w.body.Write(p)
}

func view(p *C.uint8_t, n C.size_t) []byte {
    if p == nil || n == 0 { return nil }
    return unsafe.Slice((*byte)(unsafe.Pointer(p)), int(n))
}

func parseHeaders(raw []byte) http.Header {
    h := make(http.Header)
    for len(raw) != 0 {
        i := bytes.IndexByte(raw, '\n')
        var line []byte
        if i < 0 { line, raw = raw, nil } else { line, raw = raw[:i], raw[i+1:] }
        line = bytes.TrimSuffix(line, []byte{'\r'})
        j := bytes.IndexByte(line, ':')
        if j <= 0 { continue }
        h.Add(string(line[:j]), strings.TrimSpace(string(line[j+1:])))
    }
    return h
}

func encodeHeaders(h http.Header) []byte {
    var b bytes.Buffer
    for k, values := range h {
        for _, v := range values {
            b.WriteString(k); b.WriteByte(':'); b.WriteString(v); b.WriteByte('\n')
        }
    }
    return b.Bytes()
}

func copyOut(src []byte) (*C.uint8_t, C.size_t) {
    if len(src) == 0 { return nil, 0 }
    p := C.malloc(C.size_t(len(src)))
    if p == nil { return nil, 0 }
    copy(unsafe.Slice((*byte)(p), len(src)), src)
    return (*C.uint8_t)(p), C.size_t(len(src))
}

//export gs_dispatch
func gs_dispatch(method *C.uint8_t, methodLen C.size_t, uri *C.uint8_t, uriLen C.size_t, headers *C.uint8_t, headersLen C.size_t, body *C.uint8_t, bodyLen C.size_t, out *C.gs_response) C.int {
    if out == nil { return -1 }
    *out = C.gs_response{}

    u, err := url.ParseRequestURI(string(view(uri, uriLen)))
    if err != nil { return -2 }
    req := &http.Request{
        Method: string(view(method, methodLen)),
        URL: u,
        Header: parseHeaders(view(headers, headersLen)),
        Body: http.NoBody,
        RequestURI: u.RequestURI(),
    }
    if bodyLen != 0 {
        req.Body = ioNopCloser{bytes.NewReader(view(body, bodyLen))}
        req.ContentLength = int64(bodyLen)
    }
    req.Host = req.Header.Get("Host")

    w := &responseWriter{header: make(http.Header)}
    runtime().ServeHTTP(w, req)
    if w.status == 0 { w.status = http.StatusOK }

    headerBytes := encodeHeaders(w.header)
    out.status = C.uint32_t(w.status)
    out.headers, out.headers_len = copyOut(headerBytes)
    out.body, out.body_len = copyOut(w.body.Bytes())
    if len(headerBytes) != 0 && out.headers == nil { gs_free_response(out); return -3 }
    if w.body.Len() != 0 && out.body == nil { gs_free_response(out); return -3 }
    return 0
}

type ioNopCloser struct{ *bytes.Reader }
func (ioNopCloser) Close() error { return nil }

//export gs_free_response
func gs_free_response(out *C.gs_response) {
    if out == nil { return }
    if out.headers != nil { C.free(unsafe.Pointer(out.headers)) }
    if out.body != nil { C.free(unsafe.Pointer(out.body)) }
    *out = C.gs_response{}
}

//export gs_abi_version
func gs_abi_version() C.uint32_t { return 1 }

func main() { fmt.Sprint() }
