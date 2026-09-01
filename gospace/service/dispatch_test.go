package service

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestDispatchDoesNotChangeActiveRouter(t *testing.T) {
	s := NewWithOptions(Options{})
	if err := s.RegisterFunc("default", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("default"))
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.RegisterFunc("alternate", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("alternate"))
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Activate("default"); err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	if err := s.Dispatch("alternate", rr, httptest.NewRequest(http.MethodGet, "/", nil)); err != nil {
		t.Fatal(err)
	}
	if got := rr.Body.String(); got != "alternate" {
		t.Fatalf("dispatch body = %q, want alternate", got)
	}
	if got := s.Active(); got != "default" {
		t.Fatalf("active router = %q, want default", got)
	}
}

func TestDispatchDigestRejectsWrongArtifact(t *testing.T) {
	s := NewWithOptions(Options{})
	called := false
	if err := s.RegisterFunc("wasm-v1", func(w http.ResponseWriter, _ *http.Request) {
		called = true
		_, _ = w.Write([]byte("ok"))
	}); err != nil {
		t.Fatal(err)
	}

	// The dispatch path only needs the immutable service-level association;
	// RegisterWASM is covered separately by WASM integration tests.
	s.wasmMu.Lock()
	s.wasmDigests["wasm-v1"] = "abc123"
	s.wasmMu.Unlock()

	rr := httptest.NewRecorder()
	err := s.DispatchDigest("wasm-v1", "different", rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if !errors.Is(err, ErrRouterDigestMismatch) {
		t.Fatalf("error = %v, want ErrRouterDigestMismatch", err)
	}
	if called {
		t.Fatal("handler ran despite digest mismatch")
	}
}

func TestConcurrentDispatchSelectsPerRequestRouter(t *testing.T) {
	s := NewWithOptions(Options{})
	for _, name := range []string{"a", "b"} {
		name := name
		if err := s.RegisterFunc(name, func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(name))
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Activate("a"); err != nil {
		t.Fatal(err)
	}

	const n = 100
	var wg sync.WaitGroup
	errCh := make(chan error, n)
	for i := 0; i < n; i++ {
		name := "a"
		if i%2 == 1 {
			name = "b"
		}
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			rr := httptest.NewRecorder()
			if err := s.Dispatch(name, rr, httptest.NewRequest(http.MethodGet, "/", nil)); err != nil {
				errCh <- err
				return
			}
			if rr.Body.String() != name {
				errCh <- &dispatchBodyError{got: rr.Body.String(), want: name}
			}
		}(name)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatal(err)
	}
	if got := s.Active(); got != "a" {
		t.Fatalf("active router = %q, want a", got)
	}
}

type dispatchBodyError struct{ got, want string }

func (e *dispatchBodyError) Error() string { return "dispatch body mismatch: got " + e.got + ", want " + e.want }
