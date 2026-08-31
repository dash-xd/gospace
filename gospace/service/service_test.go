package service

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

const testControlToken = "test-control-token"

func controlRequest(method, target string) *http.Request {
	r := httptest.NewRequest(method, target, nil)
	r.Header.Set(ControlTokenHeader, testControlToken)
	return r
}

func TestRegisteredRouterCanBeActivated(t *testing.T) {
	s := NewWithOptions(Options{ControlToken: testControlToken})
	if err := s.Register("hello", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("hello"))
	})); err != nil {
		t.Fatal(err)
	}

	activateResponse := httptest.NewRecorder()
	s.ServeHTTP(activateResponse, controlRequest(http.MethodPost, "/_gospace/activate/hello"))
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

func TestControlPlaneIsHiddenWithoutToken(t *testing.T) {
	s := NewWithOptions(Options{ControlToken: testControlToken})
	response := httptest.NewRecorder()
	s.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/_gospace/routers", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNotFound)
	}
}

func TestControlPlaneCanBeDisabled(t *testing.T) {
	s := NewWithOptions(Options{})
	response := httptest.NewRecorder()
	s.ServeHTTP(response, controlRequest(http.MethodGet, "/_gospace/routers"))
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNotFound)
	}
}

func TestControlPlaneRemainsReachableWithoutActiveRouter(t *testing.T) {
	s := NewWithOptions(Options{ControlToken: testControlToken})
	response := httptest.NewRecorder()
	s.ServeHTTP(response, controlRequest(http.MethodGet, "/_gospace/routers"))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
}
