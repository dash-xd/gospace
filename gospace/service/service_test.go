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

func TestCatchallFindsDeclaredRouteWithoutRouterHint(t *testing.T) {
	s := NewWithOptions(Options{})
	if err := s.RegisterRoutes("one", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("one"))
	}), []string{"GET /one"}); err != nil {
		t.Fatal(err)
	}
	if err := s.RegisterRoutes("two", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("two"))
	}), []string{"GET /two"}); err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/two", nil))
	if rr.Code != http.StatusOK || rr.Body.String() != "two" {
		t.Fatalf("status/body = %d %q, want 200 two", rr.Code, rr.Body.String())
	}
}

func TestCatchallUsesDeclaredSpecificityInsteadOfActiveProbeOrder(t *testing.T) {
	s := NewWithOptions(Options{})
	if err := s.RegisterRoutes("subtree", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("subtree"))
	}), []string{"GET /same/"}); err != nil {
		t.Fatal(err)
	}
	if err := s.RegisterRoutes("exact", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("exact"))
	}), []string{"GET /same"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Activate("subtree"); err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/same", nil))
	if rr.Body.String() != "exact" {
		t.Fatalf("body = %q, want exact", rr.Body.String())
	}
}

func TestUnmatchedRequestDelegatesToNativeHandlerOnce(t *testing.T) {
	calls := 0
	native := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get(RouterHeader) != "" {
			t.Fatal("gospace router hint leaked into native handler")
		}
		_, _ = w.Write([]byte("native:" + r.URL.Path))
	})
	s := NewWithOptions(Options{Native: native})

	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/native", bytes.NewBufferString("payload")))
	if rr.Code != http.StatusOK || rr.Body.String() != "native:/native" {
		t.Fatalf("status/body = %d %q", rr.Code, rr.Body.String())
	}
	if calls != 1 {
		t.Fatalf("native handler calls = %d, want 1", calls)
	}
}

func TestIndexedGospaceRoutePrecedesNativeFallback(t *testing.T) {
	nativeCalls := 0
	native := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		nativeCalls++
		_, _ = w.Write([]byte("native"))
	})
	s := NewWithOptions(Options{Native: native})
	if err := s.RegisterRoutes("dynamic", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("dynamic"))
	}), []string{"POST /owned"}); err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/owned", bytes.NewBufferString("body")))
	if rr.Code != http.StatusOK || rr.Body.String() != "dynamic" {
		t.Fatalf("status/body = %d %q, want 200 dynamic", rr.Code, rr.Body.String())
	}
	if nativeCalls != 0 {
		t.Fatalf("native handler executed for indexed route: %d calls", nativeCalls)
	}
}

func TestNilNativeDefaultsToNotFound(t *testing.T) {
	s := NewWithOptions(Options{})
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/missing", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNotFound)
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
