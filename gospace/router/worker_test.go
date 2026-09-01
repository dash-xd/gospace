package router

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWorkerActivation(t *testing.T) {
	w := NewWorker()
	if err := w.Register("a", http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		_, _ = rw.Write([]byte("a"))
	})); err != nil {
		t.Fatal(err)
	}
	if err := w.Register("b", http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		_, _ = rw.Write([]byte("b"))
	})); err != nil {
		t.Fatal(err)
	}

	if err := w.Activate("a"); err != nil {
		t.Fatal(err)
	}
	assertBody(t, w, "a")

	if err := w.Activate("b"); err != nil {
		t.Fatal(err)
	}
	assertBody(t, w, "b")
}

func TestWorkerRejectsDuplicateRegistration(t *testing.T) {
	w := NewWorker()
	h := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	if err := w.Register("router-v1", h); err != nil {
		t.Fatal(err)
	}
	if err := w.Register("router-v1", h); !errors.Is(err, ErrRouterExists) {
		t.Fatalf("duplicate registration error = %v, want %v", err, ErrRouterExists)
	}
}

func TestWorkerWithoutActiveRouter(t *testing.T) {
	rr := httptest.NewRecorder()
	NewWorker().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusServiceUnavailable)
	}
}

func assertBody(t *testing.T, h http.Handler, want string) {
	t.Helper()
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if got := rr.Body.String(); got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
}
