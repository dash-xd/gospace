# gospace

`gospace` is a stable Go HTTP worker whose routers can be native Go handlers, pre-registered WASM, or WASM loaded lazily.

Normal requests do not need a router header. Gospace first tries already-registered routers. A hint is only a direct shortcut when the caller already knows the router, or when a cold request carries the WASM artifact needed to load it.

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

Once registered, this works without a hint:

```http
GET /hello
```

Gospace tries the active router first, then the remaining registered routers until one returns something other than 404.

If the caller already knows the target, it can skip discovery:

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

After registration, an ordinary request can be found through catchall routing:

```http
GET /users/42
```

## Cold WASM hint

A router that is not present yet still needs its artifact. When the caller already knows it is a cold WASM route, the request can cut directly to the loader:

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

That one request verifies, compiles, registers and executes the router. It does not globally activate it.

Afterward, the route can be discovered normally:

```http
GET /users/99
```

or selected directly:

```http
GET /users/99
X-Gospace-Router: users-v1
X-Gospace-Router-SHA256: <sha256>
```

## Resolution order

```text
X-Gospace-Router supplied
    -> direct registered router
    -> cold WASM load from this request if missing

otherwise
    -> active router
    -> remaining registered native/WASM routers
    -> 404
```

Catchall discovery must observe a candidate router's 404 before trying another router, so that path buffers the request/response while probing multiple candidates. `X-Gospace-Router` is therefore also the direct path for callers or CDNs that want to skip that discovery step.

## WASM router contract

A Go WASI router exports:

```text
gospace_alloc(size uint32) unsafe.Pointer
gospace_handle() unsafe.Pointer
gospace_response_len() uint32
```

`wasmguest` adapts an ordinary `net/http.Handler` to those exports, so router code can use Chi, `http.ServeMux`, and normal middleware.

The included Chi example builds with:

```sh
cd examples/wasm-chi
GOOS=wasip1 GOARCH=wasm \
  go build -buildmode=c-shared -o router.wasm .
```

## Explicit control API

`GOSPACE_CONTROL_TOKEN` enables:

```text
GET  /_gospace/routers
POST /_gospace/activate/<name>
POST /_gospace/wasm/<name>
```

These are management/prewarming APIs. Normal route discovery does not require a registration request first.

## Server binary

```sh
cd gospace
go build -o gospace ./cmd/api
```

```sh
./gospace -port 6060
```

or, for `pyspace-minimal`:

```sh
./gospace -unix-socket /tmp/pyspace/gospace.sock
```

## Pyspace Gen 1 host

`dash-xd/pyspace-minimal` can supervise gospace under Python 3.12 Gen 1.

A normal warm request can simply flow through both catchalls:

```text
request
  -> pyspace Python route lookup
  -> gospace fallback
  -> gospace registered router lookup
```

If both layers are cold and the caller already knows the destination, the same request may additionally carry the pyspace executable hint and gospace WASM hint. Those headers accelerate/enable cold resolution; they are not required once the relevant routes are already discoverable locally.
