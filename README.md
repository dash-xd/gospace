# gospace

`gospace` is a generic Go HTTP runtime for a deployment-composed native `http.Handler` plus pre-registered or lazily loaded WASM routers.

The stock runtime contains no application-specific native routers. A build may supply one ordinary Go handler, or `nil` for no native application at all.

## Native composition

The native contract is deliberately just `net/http`:

```go
app := server.New(nativeHandler)
```

`nativeHandler` may be any `http.Handler`: `http.ServeMux`, Chi, a router returned by another package, or a mux composed from many external packages. Gospace does not need names, route manifests, or application-specific registration wrappers for this side.

```go
func nativeRouter() http.Handler {
    mux := http.NewServeMux()
    mux.Handle("/news/", http.StripPrefix("/news", newsrouter.NewRouter()))
    mux.Handle("/stonks/", http.StripPrefix("/stonks", stonksrouter.NewRouter()))
    return mux
}

func main() {
    app := server.New(nativeRouter())
    log.Fatal(http.ListenAndServe(":8080", app))
}
```

The imports above belong to the deployment build, not to gospace. Packages such as `xd-dash/news/router` and `xd-dash/stonks/router` already expose ordinary `NewRouter() http.Handler` constructors, so they can be composed with normal Go mechanisms.

`server.New(nil)` is the generic/default form. It substitutes `http.NotFoundHandler()` for the native side while keeping dynamic WASM loading available.

## Request resolution

Without an explicit router hint:

```text
request
  -> gospace-managed route index
       -> registered/preloaded/runtime WASM or explicitly indexed handler
  -> miss
       -> deployment-composed native http.Handler
  -> native 404 if unmatched
```

Gospace never executes the native handler to discover ownership and never probes several native applications. The deployment's own mux performs native routing exactly once. Gospace-managed dynamic routes still use immutable `net/http.ServeMux`-style ownership metadata so WASM dispatch remains non-executing and deterministic.

With `X-Gospace-Router`, the request goes directly to that named gospace-managed router. This remains useful for dynamically loaded WASM and other explicitly registered runtime routers.

## Pre-register WASM

A build can populate WASM before serving requests:

```go
app := server.New(nativeRouter())
module, err := os.ReadFile("router.wasm")
if err != nil {
    log.Fatal(err)
}
if _, err := app.RegisterWASMRoutes(
    context.Background(),
    "users-v1",
    module,
    []string{"GET /users/{id}"},
); err != nil {
    log.Fatal(err)
}
```

The registered WASM route participates in gospace's ownership index and takes precedence over the native fallback for its declared route.

## Cold WASM hint

A missing WASM router can still be supplied and executed in one `multipart/related` request containing:

```text
application/wasm
application/vnd.gospace.routes+json
application/vnd.gospace.request+json
```

with:

```http
X-Gospace-Router: users-v1
X-Gospace-Router-SHA256: <sha256>
X-Gospace-Control-Token: <token>
```

Gospace verifies the digest, singleflights compilation by immutable `name+digest`, publishes route ownership, executes the original application request, and keeps the compiled router in the bounded instance-local cache when capacity permits.

## WASM runtime/cache

One gospace service owns one shared Wazero runtime/WASI host. Each WASM router owns a compiled module, while each request receives a fresh module instance.

The compiled-router cache defaults to 64 routers. Recency is updated on dispatch; the least-recently-used non-active router is evicted when the bound is exceeded. Active routers are pinned, and eviction waits for in-flight leases before closing compiled modules. A newly admitted cold router serves its triggering request before eviction is enforced.

## Server package

`github.com/dash-xd/gospace/server` is the intended build-time composition surface:

```go
server.New(native http.Handler) *service.Service
server.NewWithOptions(native http.Handler, options server.Options) *service.Service
```

The returned service is itself an `http.Handler` and also exposes WASM registration/activation APIs.

The shipped standalone binary is simply the empty form:

```sh
cd gospace
go build -o gospace ./cmd/api
./gospace -port 6060
```

or over the private Unix socket used by pyspace:

```sh
./gospace -unix-socket /tmp/pyspace/gospace.sock
```

The stock Gen 2 Cloud Function entrypoint likewise uses `server.New(nil)`. A composed Cloud Function should build its own entry package and call `server.New(nativeHandler)`.

## Examples

Idiomatic executable examples live under `gospace/examples/`:

```text
examples/empty/         generic runtime, no native application
examples/native/        ordinary net/http native composition
examples/preload-wasm/  pre-register a WASM router before serving
examples/external/      composition shape for external news/stonks packages
```

The external example is build-tagged because those application dependencies intentionally do not belong in gospace's module dependency graph.

The WASM guest example remains under the repository-level `examples/wasm-chi/` because it is a separate WASI module/build.

## Explicit control API

When `GOSPACE_CONTROL_TOKEN` is configured:

```text
GET  /_gospace/routers
GET  /_gospace/resolve?method=GET&path=/users/42
POST /_gospace/activate/<name>
POST /_gospace/wasm/<name>
```

`/_gospace/resolve` is a non-executing lookup for gospace-managed indexed routes. The native fallback is intentionally opaque to this resolver because its internal routing belongs to the composed Go handler itself.
