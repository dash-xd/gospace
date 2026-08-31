# gospace

`gospace` is a modular Go HTTP worker shell for Google Cloud Functions Gen 2 and ordinary `net/http` servers.

The process registers one stable native `http.Handler`. That handler can dispatch to any of three equivalent registry sources:

- native Go routers compiled into the function and registered ahead of time;
- WASM routers bundled with the deployment and pre-registered ahead of time; or
- WASM routers resolved and registered dynamically by an already-running worker.

The WASM path does not replace the Functions Framework handler, start a subprocess, or call another service. A WASM module is wrapped in a native adapter that itself implements `http.Handler`; the already-registered worker handler atomically delegates requests to whichever registered router is active.

The distinction between pre-registration and hot/lazy loading is therefore only *when the WASM bytes become available*. Once registered, both are the same kind of WASM-backed `http.Handler` inside the worker.

## Native router registration

Native packages register ordinary `net/http` handlers against the shared registry:

```go
package myrouter

import (
    "net/http"

    "github.com/dash-xd/gospace/registry"
)

func init() {
    if err := registry.Register("my-router-v1", newRouter()); err != nil {
        panic(err)
    }
}

func newRouter() http.Handler {
    mux := http.NewServeMux()
    mux.HandleFunc("GET /hello", func(w http.ResponseWriter, _ *http.Request) {
        _, _ = w.Write([]byte("hello\n"))
    })
    return mux
}
```

The package still has to be imported by the Go source being built. This is the normal build-time composition path and is appropriate when the managed Gen 2 buildpack is already compiling the uploaded working directory or source bucket.

The checked-in worker pre-registers the existing `util` and `token` routers. Registrations are immutable: use a new/versioned name for a new implementation, then atomically activate it. This keeps in-flight requests on the handler they already acquired.

## Pre-register a WASM router

WASM routers can also be shipped with the Cloud Function source and registered during Go package initialization, just like native routers. A convenient pattern is `go:embed`:

```go
package bundledrouter

import (
    _ "embed"

    "github.com/dash-xd/gospace/registry"
)

//go:embed router.wasm
var module []byte

func init() {
    if _, err := registry.RegisterWASM("bundled-router-v1", module); err != nil {
        panic(err)
    }
}
```

The module is then available to `registry.Activate("bundled-router-v1")` immediately when that worker instance starts. This keeps the earlier gospace registration model while allowing the registered application to be a WASM router rather than a native imported Go router.

Pre-registration is useful when a router is known at deployment time but you still want the WASM isolation/interface. It is not required for routers that may appear later.

## WASM router contract

A WASM router is a Go WASI reactor that exports three low-level functions:

- `gospace_alloc(size uint32) unsafe.Pointer`
- `gospace_handle() unsafe.Pointer`
- `gospace_response_len() uint32`

This is the gospace WASM application binary interface (ABI). That use of “ABI” is unrelated to the name Huram Abi in the surrounding project family; Huram Abi refers to the master-builder name/title, not the computing acronym.

The pointer-bearing exports deliberately use `unsafe.Pointer` rather than manually converting Go pointers to integers. Go's `go:wasmexport` calling convention translates `unsafe.Pointer` to the WebAssembly host's `i32` linear-memory offset.

Router authors do not implement the wire protocol themselves. `wasmguest` adapts an ordinary `net/http.Handler` to those exports, so the application can still use Chi, `http.ServeMux`, middleware, and normal Go handler code.

The host serializes the HTTP request into the version-one `wasmhttp.Request` format. The guest reconstructs an ordinary `*http.Request`, executes its `http.Handler`, and returns a `wasmhttp.Response`.

Each request gets a fresh WASM module instance from one compiled module. This keeps mutable guest state isolated across concurrent Gen 2 invocations while avoiding recompiling the WASM bytes for every request.

The Wazero runtime is configured with request-context cancellation and a memory-page cap. The WASM guest receives no filesystem or network capability from gospace by default.

The version-one WASM HTTP bridge is intentionally buffered: request and response bodies cross the guest boundary as complete byte slices. Ordinary routing, JSON APIs, middleware, headers, status codes, and request bodies work, but streaming-only `net/http` capabilities such as `http.Flusher`, connection hijacking/WebSockets, and SSE are not exposed to WASM routers yet. Native pre-registered routers remain ordinary Go handlers and retain the capabilities supplied by the underlying runtime.

## Build the Chi example

The full worker and example use Go 1.26, matching the managed Gen 2 `go126` runtime and the current Wazero dependency floor.

```sh
cd examples/wasm-chi
go mod tidy
GOOS=wasip1 GOARCH=wasm \
  go build -buildmode=c-shared -o router.wasm .
```

The resulting `router.wasm` contains the Chi router and its Go dependencies. It is independent of the native gospace build.

## Runtime hot/lazy loading

Runtime HTTP control is disabled unless `GOSPACE_CONTROL_TOKEN` is set. The token intentionally uses `X-Gospace-Control-Token`, not `Authorization`, so Google invocation identity remains separate from worker-management authorization.

```sh
cd gospace
export GOSPACE_CONTROL_TOKEN="$(openssl rand -hex 32)"
go run ./cmd/api -port 6060
```

The privileged control surface lives under `/_gospace/`; application paths are delegated to the active router.

Load and immediately activate a WASM router:

```sh
curl --fail-with-body \
  -X POST \
  -H "X-Gospace-Control-Token: ${GOSPACE_CONTROL_TOKEN}" \
  -H 'Content-Type: application/wasm' \
  --data-binary @../examples/wasm-chi/router.wasm \
  'http://127.0.0.1:6060/_gospace/wasm/chi-v1?activate=true'
```

Then call the dynamically registered router through the same server:

```sh
curl --fail-with-body http://127.0.0.1:6060/users/42
```

Switch to any other registered router, native or WASM, without restarting the server:

```sh
curl --fail-with-body -X POST \
  -H "X-Gospace-Control-Token: ${GOSPACE_CONTROL_TOKEN}" \
  http://127.0.0.1:6060/_gospace/activate/util
```

List the worker's registered routers:

```sh
curl --fail-with-body \
  -H "X-Gospace-Control-Token: ${GOSPACE_CONTROL_TOKEN}" \
  http://127.0.0.1:6060/_gospace/routers
```

Code that already has trusted module bytes can bypass the HTTP control surface entirely and call `registry.LoadWASM` directly.

## Google Cloud Functions Gen 2

`gospace/function.go` exposes the exact bare HTTP function signature expected by Google's Go Functions buildpack:

```go
var Main func(http.ResponseWriter, *http.Request)
```

That preserves the `gospace-minimal` idiom: Google builds the uploaded source directory; consumers do not need to prebuild the native Go application themselves. The full `gospace` worker adds runtime module registration while retaining that same managed Gen 2 build contract.

For a direct deployment of this repository's `gospace` module, the source directory is the module root and the entry point is `Main`:

```sh
gcloud functions deploy gospace-worker \
  --gen2 \
  --runtime=go126 \
  --source=./gospace \
  --entry-point=Main \
  --set-env-vars=GOSPACE_CONTROL_TOKEN=... \
  --no-allow-unauthenticated
```

Prefer Secret Manager-backed configuration for a real control credential rather than committing or logging it. Use the project's normal WIF/IAM deployment path rather than treating the example command as an authentication policy.

### Instance-local registration is a cache

WASM registration is intentionally instance-local and disposable. Cloud Functions Gen 2 owns instance scheduling, may replace warm instances, and may scale to multiple instances.

That does not require pinning the function to one instance. A router resolver can treat each worker's registry as a local cache:

```text
instance A needs foo -> resolve foo.wasm -> register -> serve -> reuse while warm
instance B needs foo -> resolve foo.wasm -> register -> serve -> reuse while warm
instance C needs bar -> resolve bar.wasm -> register -> serve -> reuse while warm
```

If an instance disappears, only its local registration cache disappears. A replacement instance lazily resolves the router it needs on its next request. Router artifacts and router identity should therefore be externally resolvable; instance-local registration does not need to be durable or coordinated unless a higher-level policy specifically requires it.

Pre-registered WASM routers are simply the eager version of the same model: their bytes are already present in the deployment and registered when the instance initializes. Dynamically supplied routers are the lazy version.

This is intentionally different from an API gateway: application requests execute inside the same managed function process that owns the stable gospace handler.

## Security boundary

Treat a WASM router as executable code even though Wazero sandboxes it. Gospace caps module size, request/response size, and WASM memory and enables context-driven termination. The guest is not granted host filesystem or network access by default.

The runtime HTTP control surface is hidden unless a control token is configured, and every control endpoint requires that separate token. Platform IAM should still protect invocation independently.

For stronger artifact provenance, load immutable router artifacts and verify an expected SHA-256 or signature before activation. The load response returns the module SHA-256 so a controller can bind an assignment to an exact artifact.
