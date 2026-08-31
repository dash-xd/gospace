# gospace

`gospace` is a Go HTTP worker shell for Google Cloud Functions Gen 2 and ordinary `net/http` servers.

The process registers one stable host `http.Handler`. That handler can dispatch to either:

- native Go routers compiled into the function and registered ahead of time; or
- WebAssembly routers loaded into the already-running worker.

The WASM path does not replace the Functions Framework handler and does not start a subprocess or another service. The host handler remains live and delegates requests to a WASM-backed `http.Handler` adapter.

## Native router registration

Native packages register ordinary `net/http` handlers against the shared registry:

```go
package myrouter

import (
    "net/http"

    "github.com/dash-xd/gospace/registry"
)

func init() {
    _ = registry.Register("my-router", newRouter())
}

func newRouter() http.Handler {
    mux := http.NewServeMux()
    mux.HandleFunc("GET /hello", func(w http.ResponseWriter, _ *http.Request) {
        _, _ = w.Write([]byte("hello\n"))
    })
    return mux
}
```

The package still has to be imported by the Go source being built. This is the normal build-time composition path and is appropriate when the Gen 2 buildpack is already compiling the uploaded working directory.

The checked-in worker pre-registers the existing `util` and `token` routers.

## WASM router contract

A WASM router is a Go WASI reactor that exports only two low-level functions:

- `gospace_alloc(size uint32) unsafe.Pointer`
- `gospace_handle() uint64`

Router authors do not need to implement the wire protocol themselves. `wasmguest` adapts an ordinary `net/http.Handler` to those exports, so the application can still use Chi, `http.ServeMux`, middleware, and normal Go handler code.

The host serializes the HTTP request into the version-one `wasmhttp.Request` format. The guest reconstructs an ordinary `*http.Request`, executes its `http.Handler`, and returns a `wasmhttp.Response`.

Each request gets a fresh WASM module instance from one compiled module. This keeps mutable guest state isolated across concurrent Gen 2 invocations while avoiding recompiling the WASM bytes for every request.

The Wazero runtime is configured with request-context cancellation and a memory-page cap. The WASM guest receives no filesystem or network capability from gospace by default.

## Build the Chi example

Go 1.24 or newer is required to build Go WASI reactors using `go:wasmexport` and `-buildmode=c-shared`.

```sh
cd examples/wasm-chi
go mod tidy
GOOS=wasip1 GOARCH=wasm \
  go build -buildmode=c-shared -o router.wasm .
```

The resulting `router.wasm` contains the Chi router and its Go dependencies. It is independent of the native gospace build.

## Run locally

```sh
cd gospace
go mod tidy
go run ./cmd/api -port 6060
```

The control surface lives under `/_gospace/`; application paths are delegated to the active router.

Load and immediately activate a WASM router:

```sh
curl --fail-with-body \
  -X POST \
  -H 'Content-Type: application/wasm' \
  --data-binary @../examples/wasm-chi/router.wasm \
  'http://127.0.0.1:6060/_gospace/wasm/chi?activate=true'
```

Then call the hot-loaded router through the same server:

```sh
curl --fail-with-body http://127.0.0.1:6060/users/42
```

Switch to a pre-registered native router without restarting the server:

```sh
curl --fail-with-body -X POST \
  http://127.0.0.1:6060/_gospace/activate/util
```

List the worker's registered routers:

```sh
curl --fail-with-body http://127.0.0.1:6060/_gospace/routers
```

## Google Cloud Functions Gen 2

`gospace/function.go` exposes the exact bare HTTP function signature expected by Google's Go Functions buildpack:

```go
var Main func(http.ResponseWriter, *http.Request)
```

That keeps the same idiom used by `gospace-minimal`: Google builds the uploaded source directory; consumers do not need to prebuild the native Go application themselves.

For a direct deployment of this repository's `gospace` module, the source directory is the module root and the entry point is `Main`:

```sh
gcloud functions deploy gospace-worker \
  --gen2 \
  --runtime=go126 \
  --source=./gospace \
  --entry-point=Main \
  --max-instances=1 \
  --no-allow-unauthenticated
```

Use the project's normal WIF/IAM deployment path rather than treating the example command as an authentication policy.

### Instance-local hot loading

WASM registration is process-local. Cloud Functions Gen 2 owns instance scheduling and may replace an instance at any time. It may also run multiple instances if scaling is allowed.

For a worker intended to retain one hot-loaded router in memory, constrain that function to one active instance (`--max-instances=1`). Even then, a platform restart loses the in-memory module and it must be loaded again. A later durable assignment/rehydration layer can restore a router after restart without changing the native gospace deployment.

This is intentionally different from an API gateway: application requests execute inside the same managed function process that owns the stable gospace handler.

## Security boundary

Treat a WASM router as executable code even though Wazero sandboxes it. The loader caps module size, request/response size, and WASM memory, but the control endpoints still need the same invocation authorization as any other privileged worker-management operation.

For production use, load immutable router artifacts and verify an expected SHA-256 or signature before activating them. The load response already returns the module SHA-256 so an external controller can bind an assignment to an exact artifact.
