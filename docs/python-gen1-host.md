# Running gospace as a supervised binary

The `gospace/cmd/api` worker is also an ordinary executable. It does not require the Google Go Functions runtime when it is hosted behind another HTTP process.

For the Python 3.12 Gen 1 compatibility path, build an exact gospace revision as a static Linux binary during deployment composition:

```sh
cd gospace
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -trimpath -o ../bin/gospace ./cmd/api
```

The worker can listen on a Unix-domain socket instead of allocating an internal TCP port:

```sh
./bin/gospace --unix-socket /tmp/pyspace/gospace.sock
```

`dash-xd/pyspace-minimal`'s request-driven supervisor uses this mode. Pyspace owns the child-process lifecycle and HTTP proxying; gospace still owns native/WASM registration, per-request named dispatch, Wazero compilation/instantiation, and router execution.

A composed deployment should bind the pyspace application registration to the SHA-256 of the exact gospace binary. The pyspace supervisor rechecks that digest whenever it must spawn the process, so replacing a file in place cannot silently change the executable behind an immutable registered application name.

The intended lifecycle is:

```text
cold Python instance
    -> request selects gospace
    -> pyspace verifies binary digest
    -> spawn gospace on a Unix socket
    -> proxy request

warm Python instance
    -> request selects gospace
    -> reuse healthy child
    -> proxy request

child exits while instance remains warm
    -> next request verifies the same executable
    -> respawn

Google replaces the instance
    -> Python registry and gospace child disappear together
    -> replacement reconstructs lazily
```

This is a compatibility host, not a durability mechanism. Authoritative router artifacts and assignments remain external to the instance-local process/registration caches.
