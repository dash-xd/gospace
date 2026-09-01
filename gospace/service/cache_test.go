package service

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dash-xd/gospace/router"
)

func TestLoadGroupCollapsesConcurrentLoads(t *testing.T) {
	var group loadGroup
	var calls atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})

	const workers = 12
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			_, _, err := group.Do("router:digest", func() (loadResult, error) {
				if calls.Add(1) == 1 {
					close(started)
				}
				<-release
				return loadResult{digest: "digest", loaded: true}, nil
			})
			if err != nil {
				t.Errorf("Do error: %v", err)
			}
		}()
	}

	<-started
	time.Sleep(20 * time.Millisecond)
	close(release)
	wg.Wait()
	if got := calls.Load(); got != 1 {
		t.Fatalf("compile function called %d times, want 1", got)
	}
}

type closeTrackingHandler struct {
	closed atomic.Bool
	calls  atomic.Int32
}

func (h *closeTrackingHandler) ServeHTTP(http.ResponseWriter, *http.Request) {
	h.calls.Add(1)
}
func (h *closeTrackingHandler) Close() error {
	h.closed.Store(true)
	return nil
}

func TestBoundedWASMCacheEvictsOldestNonActiveRouter(t *testing.T) {
	s := &Service{
		worker:          router.NewWorker(),
		maxWASMRouters: 1,
		wasm:            newWASMRegistry(),
	}
	old := &closeTrackingHandler{}
	newer := &closeTrackingHandler{}
	if err := s.worker.Register("old", old); err != nil {
		t.Fatal(err)
	}
	if err := s.worker.Register("new", newer); err != nil {
		t.Fatal(err)
	}
	s.wasm.entries["old"] = &wasmEntry{digest: "old", lastUsed: 1}
	s.wasm.entries["new"] = &wasmEntry{digest: "new", lastUsed: 2}

	s.evictWASMIfNeeded()
	if _, ok := s.worker.Handler("old"); ok {
		t.Fatal("old router was not evicted")
	}
	if _, ok := s.worker.Handler("new"); !ok {
		t.Fatal("new router was unexpectedly evicted")
	}
	if !old.closed.Load() {
		t.Fatal("evicted router was not closed")
	}
}

func TestBoundedWASMCacheDoesNotEvictActiveRouter(t *testing.T) {
	s := &Service{
		worker:          router.NewWorker(),
		maxWASMRouters: 1,
		wasm:            newWASMRegistry(),
	}
	active := &closeTrackingHandler{}
	other := &closeTrackingHandler{}
	if err := s.worker.Register("active", active); err != nil {
		t.Fatal(err)
	}
	if err := s.worker.Register("other", other); err != nil {
		t.Fatal(err)
	}
	if err := s.worker.Activate("active"); err != nil {
		t.Fatal(err)
	}
	s.wasm.entries["active"] = &wasmEntry{digest: "active", lastUsed: 1}
	s.wasm.entries["other"] = &wasmEntry{digest: "other", lastUsed: 2}

	s.evictWASMIfNeeded()
	if _, ok := s.worker.Handler("active"); !ok {
		t.Fatal("active router was evicted")
	}
	if _, ok := s.worker.Handler("other"); ok {
		t.Fatal("non-active router was not evicted")
	}
	if active.closed.Load() {
		t.Fatal("active router was closed")
	}
}

func TestColdAdmissionServesTriggeringRequestBeforeEviction(t *testing.T) {
	s := &Service{
		worker:          router.NewWorker(),
		maxWASMRouters: 1,
		wasm:            newWASMRegistry(),
	}
	active := &closeTrackingHandler{}
	cold := &closeTrackingHandler{}
	if err := s.worker.Register("active", active); err != nil {
		t.Fatal(err)
	}
	if err := s.worker.Register("cold", cold); err != nil {
		t.Fatal(err)
	}
	if err := s.worker.Activate("active"); err != nil {
		t.Fatal(err)
	}
	s.wasm.entries["active"] = &wasmEntry{digest: "active", lastUsed: 1}
	s.wasm.entries["cold"] = &wasmEntry{digest: "cold", lastUsed: 2}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/cold", nil)
	if err := s.dispatchThenEvict("cold", rr, req); err != nil {
		t.Fatal(err)
	}
	if got := cold.calls.Load(); got != 1 {
		t.Fatalf("cold router executed %d times, want 1", got)
	}
	if _, ok := s.worker.Handler("active"); !ok {
		t.Fatal("active router was unexpectedly evicted")
	}
	if _, ok := s.worker.Handler("cold"); ok {
		t.Fatal("cold router should be evicted after its triggering request")
	}
	if !cold.closed.Load() {
		t.Fatal("cold router was not closed after post-dispatch eviction")
	}
}

func TestActivationPinsNewRouterBeforeEviction(t *testing.T) {
	s := &Service{
		worker:          router.NewWorker(),
		maxWASMRouters: 1,
		wasm:            newWASMRegistry(),
	}
	old := &closeTrackingHandler{}
	newer := &closeTrackingHandler{}
	if err := s.worker.Register("old", old); err != nil {
		t.Fatal(err)
	}
	if err := s.worker.Register("new", newer); err != nil {
		t.Fatal(err)
	}
	if err := s.worker.Activate("old"); err != nil {
		t.Fatal(err)
	}
	s.wasm.entries["old"] = &wasmEntry{digest: "old", lastUsed: 1}
	s.wasm.entries["new"] = &wasmEntry{digest: "new", lastUsed: 2}

	if err := s.activateThenEvict("new"); err != nil {
		t.Fatal(err)
	}
	if got := s.worker.Active(); got != "new" {
		t.Fatalf("active = %q, want new", got)
	}
	if _, ok := s.worker.Handler("new"); !ok {
		t.Fatal("new active router was evicted")
	}
	if _, ok := s.worker.Handler("old"); ok {
		t.Fatal("old non-active router was not evicted")
	}
	if !old.closed.Load() {
		t.Fatal("old router was not closed")
	}
}
