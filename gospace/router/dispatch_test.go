package router

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestServeRouterDoesNotMutateActive(t *testing.T) {
	w := NewWorker()
	if err := w.RegisterFunc("a", func(rw http.ResponseWriter, _ *http.Request) { _, _ = rw.Write([]byte("a")) }); err != nil {
		t.Fatal(err)
	}
	if err := w.RegisterFunc("b", func(rw http.ResponseWriter, _ *http.Request) { _, _ = rw.Write([]byte("b")) }); err != nil {
		t.Fatal(err)
	}
	if err := w.Activate("a"); err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	if err := w.ServeRouter("b", rr, httptest.NewRequest(http.MethodGet, "/", nil)); err != nil {
		t.Fatal(err)
	}
	if got := rr.Body.String(); got != "b" {
		t.Fatalf("body = %q, want b", got)
	}
	if got := w.Active(); got != "a" {
		t.Fatalf("active = %q, want a", got)
	}
}
