package router

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestMatchDoesNotExecuteCandidateHandlers(t *testing.T) {
	w := NewWorker()
	var aCalls atomic.Int32
	var bCalls atomic.Int32
	if err := w.RegisterRoutes("a", http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		aCalls.Add(1)
		_, _ = rw.Write([]byte("a"))
	}), []string{"GET /a/{id}"}); err != nil {
		t.Fatal(err)
	}
	if err := w.RegisterRoutes("b", http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		bCalls.Add(1)
		_, _ = rw.Write([]byte("b"))
	}), []string{"GET /b/{id}"}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/b/7", nil)
	name, ok := w.Match(req)
	if !ok || name != "b" {
		t.Fatalf("Match = %q, %v; want b, true", name, ok)
	}
	if aCalls.Load() != 0 || bCalls.Load() != 0 {
		t.Fatalf("route matching executed handlers: a=%d b=%d", aCalls.Load(), bCalls.Load())
	}

	rr := httptest.NewRecorder()
	if err := w.ServeRouter(name, rr, req); err != nil {
		t.Fatal(err)
	}
	if aCalls.Load() != 0 || bCalls.Load() != 1 {
		t.Fatalf("dispatch calls: a=%d b=%d", aCalls.Load(), bCalls.Load())
	}
}

func TestRouteIndexIsMethodAware(t *testing.T) {
	w := NewWorker()
	if err := w.RegisterRoutes("users", http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), []string{"GET /users/{id}"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := w.Match(httptest.NewRequest(http.MethodPost, "/users/1", nil)); ok {
		t.Fatal("POST matched GET-only route metadata")
	}
}

func TestRouteIndexUsesServeMuxSpecificity(t *testing.T) {
	w := NewWorker()
	if err := w.RegisterRoutes("users", http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), []string{"GET /users/{id}"}); err != nil {
		t.Fatal(err)
	}
	if err := w.RegisterRoutes("admin", http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), []string{"GET /users/admin"}); err != nil {
		t.Fatal(err)
	}
	name, ok := w.Match(httptest.NewRequest(http.MethodGet, "/users/admin", nil))
	if !ok || name != "admin" {
		t.Fatalf("Match = %q, %v; want admin, true", name, ok)
	}
}

func TestRouteIndexRejectsConflictingOwnership(t *testing.T) {
	w := NewWorker()
	h := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	if err := w.RegisterRoutes("one", h, []string{"GET /same"}); err != nil {
		t.Fatal(err)
	}
	if err := w.RegisterRoutes("two", h, []string{"GET /same"}); err == nil {
		t.Fatal("duplicate route ownership was accepted")
	}
}

func TestRemoveDropsRouteBeforeReturningHandler(t *testing.T) {
	w := NewWorker()
	h := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	if err := w.RegisterRoutes("gone", h, []string{"GET /gone"}); err != nil {
		t.Fatal(err)
	}
	removed, err := w.Remove("gone")
	if err != nil {
		t.Fatal(err)
	}
	if removed == nil {
		t.Fatal("Remove returned nil handler")
	}
	if _, ok := w.Match(httptest.NewRequest(http.MethodGet, "/gone", nil)); ok {
		t.Fatal("removed router remained in route index")
	}
}
