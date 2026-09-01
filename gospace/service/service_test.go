package service

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"testing"

	"github.com/dash-xd/gospace/wasmhttp"
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

func TestHintedRequestDispatchesNamedRouterWithoutActivation(t *testing.T) {
	s := NewWithOptions(Options{})
	if err := s.Register("hinted", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(RouterHeader) != "" {
			t.Fatal("router hint leaked into application request")
		}
		_, _ = w.Write([]byte(r.URL.Path))
	})); err != nil {
		t.Fatal(err)
	}

	r := httptest.NewRequest(http.MethodPost, "/payload", bytes.NewBufferString("body"))
	r.Header.Set(RouterHeader, "hinted")
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, r)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if got := rr.Body.String(); got != "/payload" {
		t.Fatalf("body = %q, want /payload", got)
	}
	if s.Active() != "" {
		t.Fatalf("hinted dispatch changed active router to %q", s.Active())
	}
}

func TestColdHintIsOneRequestAndRequiresAuthorization(t *testing.T) {
	s := NewWithOptions(Options{ControlToken: testControlToken})

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	h := make(textproto.MIMEHeader)
	h.Set("Content-Type", requestPartType)
	part, err := mw.CreatePart(h)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.NewEncoder(part).Encode(wasmhttp.Request{Method: http.MethodPost, URL: "/payload", Body: []byte("hello")}); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}

	r := httptest.NewRequest(http.MethodPost, "/", &body)
	r.Header.Set(RouterHeader, "missing-v1")
	r.Header.Set(RouterDigestHeader, "deadbeef")
	r.Header.Set("Content-Type", "multipart/related; boundary="+mw.Boundary())
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, r)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNotFound)
	}
}

func TestRegisterWASMUsesServiceModuleLimit(t *testing.T) {
	s := NewWithOptions(Options{MaxWASMBytes: 4})
	if _, err := s.RegisterWASM(context.Background(), "bundled-v1", make([]byte, 5)); err == nil {
		t.Fatal("RegisterWASM accepted module larger than service limit")
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
