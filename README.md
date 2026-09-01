# gospace

`gospace` is a stable Go HTTP worker whose routers can be native Go handlers, pre-registered WASM, or WASM loaded lazily.

Normal requests do not need a router header. Routers that want to participate in no-header discovery publish immutable `net/http.ServeMux`-style route metadata at registration time. Gospace matches that metadata without executing candidate application handlers, then executes exactly one selected router. A hint remains an optional direct shortcut when the caller already knows the router, or when a cold request carries the WASM artifact needed to load it.

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
    if err := registry.RegisterRoutes("hello-v1", mux, []string{"GET /hello"}); err != nil {
        panic(err)
    }
}
```

Once registered, this works without a hint:

```http
GET /hello
```

Route ownership is resolved from the declared patterns only; gospace does not call unrelated handlers and interpret their 404s as misses. Conflicting ownership metadata is rejected during registration using Go `http.ServeMux` pattern semantics.

If the caller already knows the target, it can bypass the route index:

```http
GET /hello
X-Gospace-Router: hello-v1
```

`Register` remains available for direct-only routers that should not participate in catchall discovery.

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
    if _, err := registry.RegisterWASMRoutes(
        "users-v1",
        module,
        []string{"GET /users/{id}"},
    ); err != nil {
        panic(err)
    }
}
```

After registration, an ordinary request is resolved through the same route index:

```http
GET /users/42
```

`RegisterWASM` remains the direct-only form when no route metadata is desired.

## Cold WASM hint

A router that is not present yet still needs its artifact. When the caller already knows it is a cold WASM route, one `multipart/related` request can carry the raw module, immutable route manifest, and application request:

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
Content-Type: application/vnd.gospace.routes+json

{"patterns":["GET /users/{id}"]}
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

That one request verifies the SHA-256, singleflights concurrent compilation for the same immutable `name+digest`, publishes route ownership, and executes the original application request. It does not globally activate the router.

Afterward, while the compiled router remains in the instance-local cache, the route can be discovered normally:

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
    -> non-executing route-index match
    -> exactly one native/WASM router
    -> 404
```

The no-header path does not buffer or replay the request body and does not execute speculative middleware. Method-specific and parameterized patterns use Go `http.ServeMux` matching semantics.

## WASM runtime and cache

A gospace service owns one process-level Wazero runtime and shared WASI host. Each registered WASM router owns a compiled module, while each HTTP request still receives a fresh isolated module instance.

Cold compilation is collapsed by immutable `router-name + SHA-256` so concurrent requests for the same artifact share one compilation. The instance-local compiled-router cache is bounded (`64` routers by default). Recency is tracked on dispatch; the least-recently-used non-active WASM router is evicted when capacity is exceeded. Active routers are pinned, and eviction waits for in-flight request leases before closing a compiled module.

A cold router always serves the request that admitted it before post-dispatch eviction can remove it. Likewise, `activate=true` establishes the new active pin before capacity enforcement runs.

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
GET  /_gospace/resolve?method=GET&path=/users/42
POST /_gospace/activate/<name>
POST /_gospace/wasm/<name>
```

`/_gospace/resolve` performs a protected non-executing ownership lookup. Pyspace uses it only when it has multiple gospace backends and must decide which child owns a request before application execution.

These are management/prewarming/resolution APIs. Normal route discovery does not require a separate registration request first.

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

With one gospace backend, pyspace forwards a Python-route miss once and lets gospace's local route index resolve ownership. With multiple gospace backends, pyspace first asks each private `/_gospace/resolve` endpoint so only the owning application process executes the request.

```text
                     optional upstream/CDN hint
                              |
                              v
request -----------------> pyspace
                              |
                    Python route lookup
                              |
                         miss |
                              v
                       gospace UDS
                              |
                    gospace route index
                       /           \
                 native           WASM
                                   |
                             compiled cache
                                   |
                          shared Wazero runtime
                                   |
                         fresh module/request
```

If both layers are cold and the caller already knows the destination, the same request may additionally carry the pyspace executable hint and gospace WASM artifact hint. Those hints accelerate or enable cold resolution; they are not required once the relevant routes are locally indexed.
