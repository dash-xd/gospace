# Embedded gospace ABI

`cmd/cshared` builds gospace as an in-process C ABI for Python or Node hosts.

Build from `gospace/`:

```sh
go build -buildmode=c-shared -trimpath -ldflags='-s -w' -o libgospace.so ./cmd/cshared
```

ABI v1 exports `gs_abi_version`, `gs_dispatch`, and `gs_free_response`. Requests use borrowed byte spans; headers use `Name:Value\n`. Responses are library-allocated and released with `gs_free_response`. No Go pointer crosses the ownership boundary.

The runtime initializes once per process. WASM loading, digest validation, router caching, and hinted dispatch remain inside gospace.

For deployment-native routers, a build composer adds a generated Go file in `cmd/cshared` whose `init` replaces `deploymentRegister` and calls `s.RegisterRoutes(...)`. This keeps native routers statically linked while retaining OTA WASM for routers absent at build time.
