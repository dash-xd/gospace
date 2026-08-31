package service

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRegisteredRouterCanBeActivated(t *testing.T) {
	s := New()
	if err := s.Register("hello", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("hello"))
	})); err != nil {
		t.Fatal(err)
	}

	activate := httptest.NewRequest(http.MethodPost, "/_gospace/activate/hello", nil)
	activateResponse := httptest.NewRecorder()
	s.ServeHTTP(activateResponse, activate)
	if activateResponse.Code != http.StatusOK {
		t.Fatalf("activate status = %d, want %d", activateResponse.Code, http.StatusOK)
	}

	request := httptest.NewRequest(http.MethodGet, "/anything", nil)
	response := httptest.NewRecorder()
	s.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("router status = %d, want %d", response.Code, http.StatusOK)
	}
	if got := response.Body.String(); got != "hello" {
		t.Fatalf("body = %q, want hello", got)
	}
}

func TestControlPlaneRemainsReachableWithoutActiveRouter(t *testing.T) {
	s := New()
	response := httptest.NewRecorder()
	s.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/_gospace/routers", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
}
