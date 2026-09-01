package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"

	"github.com/dash-xd/gospace/router"
	wasmrouter "github.com/dash-xd/gospace/wasm"
)

var ErrRouterDigestMismatch = errors.New("router digest mismatch")
var ErrRouterRoutesMismatch = errors.New("router routes mismatch")

type wasmEntry struct {
	digest   string
	handler  *wasmrouter.Handler
	routes   []string
	lastUsed uint64
}

type wasmRegistry struct {
	mu      sync.RWMutex
	entries map[string]*wasmEntry
	clock   uint64
}

func newWASMRegistry() wasmRegistry {
	return wasmRegistry{entries: make(map[string]*wasmEntry)}
}

type loadResult struct {
	digest string
	loaded bool
}

type loadCall struct {
	done   chan struct{}
	result loadResult
	err    error
}

type loadGroup struct {
	mu    sync.Mutex
	calls map[string]*loadCall
}

func (g *loadGroup) Do(key string, fn func() (loadResult, error)) (loadResult, bool, error) {
	g.mu.Lock()
	if g.calls == nil {
		g.calls = make(map[string]*loadCall)
	}
	if call, ok := g.calls[key]; ok {
		g.mu.Unlock()
		<-call.done
		return call.result, true, call.err
	}
	call := &loadCall{done: make(chan struct{})}
	g.calls[key] = call
	g.mu.Unlock()

	call.result, call.err = fn()
	close(call.done)
	g.mu.Lock()
	delete(g.calls, key)
	g.mu.Unlock()
	return call.result, false, call.err
}

func wasmDigest(module []byte) string {
	sum := sha256.Sum256(module)
	return hex.EncodeToString(sum[:])
}

func (s *Service) registerWASM(ctx context.Context, name string, module []byte, patterns []string) (string, error) {
	if name == "" {
		return "", errors.New("router name is required")
	}
	if int64(len(module)) > s.maxWASM {
		return "", fmt.Errorf("WASM module exceeds %d bytes", s.maxWASM)
	}
	if s.wasmEngineErr != nil {
		return "", s.wasmEngineErr
	}
	if _, ok := s.worker.Handler(name); ok {
		return "", router.ErrRouterExists
	}

	digest := wasmDigest(module)
	h, err := s.wasmEngine.Compile(ctx, module, wasmrouter.Options{})
	if err != nil {
		return "", err
	}
	if err := s.worker.RegisterRoutes(name, h, patterns); err != nil {
		_ = h.Close()
		return "", err
	}

	s.wasm.mu.Lock()
	s.wasm.clock++
	s.wasm.entries[name] = &wasmEntry{
		digest:   digest,
		handler:  h,
		routes:   append([]string(nil), patterns...),
		lastUsed: s.wasm.clock,
	}
	s.wasm.mu.Unlock()

	s.evictWASMIfNeeded()
	return digest, nil
}

func (s *Service) WASMDigest(name string) (digest string, ok bool) {
	s.wasm.mu.RLock()
	entry, ok := s.wasm.entries[name]
	if ok {
		digest = entry.digest
	}
	s.wasm.mu.RUnlock()
	return digest, ok
}

func (s *Service) touchWASM(name string) {
	s.wasm.mu.Lock()
	if entry, ok := s.wasm.entries[name]; ok {
		s.wasm.clock++
		entry.lastUsed = s.wasm.clock
	}
	s.wasm.mu.Unlock()
}

func (s *Service) requireWASMDigest(name, expected string) error {
	if expected == "" {
		return nil
	}
	actual, ok := s.WASMDigest(name)
	if !ok || actual != expected {
		return fmt.Errorf("%w: router %q has %q, want %q", ErrRouterDigestMismatch, name, actual, expected)
	}
	return nil
}

func (s *Service) requireRoutes(name string, expected []string) error {
	if len(expected) == 0 {
		return nil
	}
	actual := s.worker.Routes(name)
	if !sameStrings(actual, expected) {
		return fmt.Errorf("%w: router %q has %v, want %v", ErrRouterRoutesMismatch, name, actual, expected)
	}
	return nil
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	aa := append([]string(nil), a...)
	bb := append([]string(nil), b...)
	sort.Strings(aa)
	sort.Strings(bb)
	for i := range aa {
		if aa[i] != bb[i] {
			return false
		}
	}
	return true
}

func (s *Service) ensureWASM(ctx context.Context, name, expectedDigest string, module []byte, patterns []string) (string, bool, error) {
	if _, ok := s.worker.Handler(name); ok {
		// A handler becomes visible immediately before its cache metadata is
		// published. Treat it as fully cached only once the digest record is also
		// visible; otherwise fall through to the name+digest singleflight below,
		// whose leader owns the publication in progress.
		if digest, published := s.WASMDigest(name); published {
			if expectedDigest != "" && digest != expectedDigest {
				return "", false, fmt.Errorf("%w: router %q has %q, want %q", ErrRouterDigestMismatch, name, digest, expectedDigest)
			}
			if err := s.requireRoutes(name, patterns); err != nil {
				return "", false, err
			}
			s.touchWASM(name)
			return digest, false, nil
		}
	}

	actual := wasmDigest(module)
	if expectedDigest != "" && actual != expectedDigest {
		return actual, false, ErrRouterDigestMismatch
	}
	key := name + ":" + actual
	result, shared, err := s.loads.Do(key, func() (loadResult, error) {
		if _, ok := s.worker.Handler(name); ok {
			if digest, published := s.WASMDigest(name); published {
				if expectedDigest != "" && digest != expectedDigest {
					return loadResult{}, fmt.Errorf("%w: router %q has %q, want %q", ErrRouterDigestMismatch, name, digest, expectedDigest)
				}
				if err := s.requireRoutes(name, patterns); err != nil {
					return loadResult{}, err
				}
				return loadResult{digest: digest}, nil
			}
		}
		digest, err := s.registerWASM(context.WithoutCancel(ctx), name, module, patterns)
		if err != nil {
			if errors.Is(err, router.ErrRouterExists) {
				if err := s.requireWASMDigest(name, expectedDigest); err != nil {
					return loadResult{}, err
				}
				if err := s.requireRoutes(name, patterns); err != nil {
					return loadResult{}, err
				}
				digest, _ = s.WASMDigest(name)
				return loadResult{digest: digest}, nil
			}
			return loadResult{}, err
		}
		return loadResult{digest: digest, loaded: true}, nil
	})
	if err != nil {
		return "", false, err
	}
	s.touchWASM(name)
	return result.digest, result.loaded && !shared, nil
}

func (s *Service) evictWASMIfNeeded() {
	for {
		s.wasm.mu.RLock()
		if len(s.wasm.entries) <= s.maxWASMRouters {
			s.wasm.mu.RUnlock()
			return
		}
		active := s.worker.Active()
		var victim string
		var oldest uint64 = ^uint64(0)
		for name, entry := range s.wasm.entries {
			if name == active {
				continue
			}
			if entry.lastUsed < oldest {
				oldest = entry.lastUsed
				victim = name
			}
		}
		s.wasm.mu.RUnlock()
		if victim == "" {
			return
		}

		handler, err := s.worker.Remove(victim)
		if err != nil {
			return
		}
		s.wasm.mu.Lock()
		delete(s.wasm.entries, victim)
		s.wasm.mu.Unlock()
		if closer, ok := handler.(interface{ Close() error }); ok {
			_ = closer.Close()
		}
	}
}

func splitRoutePatterns(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if pattern := strings.TrimSpace(part); pattern != "" {
			out = append(out, pattern)
		}
	}
	return out
}

func (s *Service) resolveTarget(r *http.Request) (string, bool) {
	return s.worker.Match(r)
}
