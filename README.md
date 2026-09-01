# gospace

`gospace` is a stable Go HTTP worker whose routers can be native Go handlers, pre-registered WASM, or WASM supplied lazily with the request that needs it.

Registration and dispatch are separate. A router can be the default via `Activate`, or one request can select a named router without changing the default.

## Native router

```go
package myrouter

import (
    "net/http"
    "github.com/dash-xd/gospace/registry"
)

func init() {
    mux := http.NewServeMux()
    mux.HandleFunc("GET /hello", func(w http.ResponseWriter, _ *http.Request) {
        _, _ = w.Write([]byte("hello"))
    })
    if err := registry.Register("hello-v1", mux); err != nil {
        panic(err)
    }
}
```

A named request can use it without activation:

```http
GET /hello
X-Gospace-Router: hello-v1
```

## Pre-register WASM

```go
package bundled

import (
    _ "embed"
    "github.com/dash-xd/gospace/registry"
)

//go:embed router.wasm
var module []byte

func init() {
    if _, err := registry.RegisterWASM("users-v1", module); err != nil {
        panic(err)
    }
}
```

## Lazy WASM: one request

The lazy path does not require `POST /_gospace/wasm/...` followed by a second application request.

A cold request carries:

- `X-Gospace-Router`: immutable router name;
- `X-Gospace-Router-SHA256`: expected WASM digest;
- `X-Gospace-Control-Token`: required only when this request actually loads code;
- `Content-Type: multipart/related` with the WASM bytes and the original HTTP request.

Example wire shape:

```http
POST /
X-Gospace-Router: users-v1
X-Gospace-Router-SHA256: <sha256>
X-Gospace-Control-Token: <GOSPACE_CONTROL_TOKEN>
Content-Type: multipart/related; boundary=gospace

--gospace
Content-Type: application/wasm

<raw router.wasm bytes>
--gospace
Content-Type: application/vnd.gospace.request+json

{
  "method": "GET",
  "url": "/users/42",
  "host": "example",
  "header": {},
  "body": null
}
--gospace--
```

On a cold instance gospace verifies the SHA-256, compiles and registers `users-v1`, reconstructs the inner request, and immediately runs `/users/42` through that router. The router is not globally activated.

Once warm, the same router can be called with an ordinary request:

```http
GET /users/99
X-Gospace-Router: users-v1
X-Gospace-Router-SHA256: <sha256>
```

No WASM bytes or control token are needed on the warm path. If the named router is absent and no multipart artifact is supplied, gospace returns `428 Precondition Required` instead of requiring a separate registration round trip.

The multipart request part uses the same `wasmhttp.Request` structure used by the host/guest bridge. Its `body` field is a JSON `[]byte`, so JSON encoding represents it as base64. The WASM artifact itself remains raw binary.

## WASM router contract

A Go WASI router exports:

```text
gospace_alloc(size uint32) unsafe.Pointer
gospace_handle() unsafe.Pointer
gospace_response_len() uint32
```

`wasmguest` adapts an ordinary `net/http.Handler` to those exports, so router code can still use Chi, `http.ServeMux`, and normal middleware.

The included Chi example builds with:

```sh
cd examples/wasm-chi
GOOS=wasip1 GOARCH=wasm \
  go build -buildmode=c-shared -o router.wasm .
```

Each request gets a fresh WASM module instance from a compiled module. The compiled handler is cached by immutable router name/digest while the worker instance remains warm.

## Explicit control API

`GOSPACE_CONTROL_TOKEN` enables the older explicit management endpoints:

```text
GET  /_gospace/routers
POST /_gospace/activate/<name>
POST /_gospace/wasm/<name>
```

Those endpoints remain useful for administration and prewarming. They are not required for lazy request dispatch.

## Server binary

```sh
cd gospace
go build -o gospace ./cmd/api
```

TCP:

```sh
./gospace -port 6060
```

Unix socket, used by `pyspace-minimal`:

```sh
./gospace -unix-socket /tmp/pyspace/gospace.sock
```

## Google Cloud Functions Gen 2

`gospace/function.go` exposes the stable Go HTTP entry point `Main`. For a direct Gen 2 deployment the `gospace` directory is the module root and `Main` is the function entry point.

Instance-local registration is intentionally a cache. If Google replaces an instance, the next self-contained request can load the required router again. No global activation or cross-instance cache coordination is required.

## Pyspace Gen 1 host

`dash-xd/pyspace-minimal` can supervise the gospace binary under a Python 3.12 Gen 1 function. The same incoming request can contain both layers of hints:

```text
X-Pyspace-App: gospace-v1
X-Pyspace-Gospace-Binary: /workspace/bin/gospace
X-Pyspace-Control-Token: ...

X-Gospace-Router: users-v1
X-Gospace-Router-SHA256: ...
X-Gospace-Control-Token: ...
```

On a fully cold instance that one request can therefore register/spawn gospace, load the WASM router, and execute the enclosed application request. On a warm instance only the two name/digest selection headers are needed.
