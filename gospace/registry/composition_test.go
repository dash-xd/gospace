package registry

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dash-xd/gospace/service"
)

func TestComposeRegistersDeclaredRouteWithoutExecutingDuringMatch(t *testing.T) {
	previous := Default
	Default = service.New()
	defer func() { Default = previous }()

	calls := 0
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_, _ = w.Write([]byte(r.URL.Path))
	})
	if err := Compose(NativeRouter{
		Name:    "external-v1",
		Handler: handler,
		Routes:  []string{"POST /external/{id}"},
	}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/external/42", nil)
	if calls != 0 {
		t.Fatalf("handler executed during composition: calls=%d", calls)
	}
	rr := httptest.NewRecorder()
	ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if calls != 1 {
		t.Fatalf("handler calls = %d, want 1", calls)
	}
}

func TestComposeAllowsDirectOnlyRouter(t *testing.T) {
	previous := Default
	Default = service.New()
	defer func() { Default = previous }()

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("direct"))
	})
	if err := Compose(NativeRouter{Name: "direct-v1", Handler: handler}); err != nil {
		t.Fatal(err)
	}

	plain := httptest.NewRecorder()
	ServeHTTP(plain, httptest.NewRequest(http.MethodGet, "/anything", nil))
	if plain.Code != http.StatusNotFound {
		t.Fatalf("unhinted status = %d, want %d", plain.Code, http.StatusNotFound)
	}

	hintedReq := httptest.NewRequest(http.MethodGet, "/anything", nil)
	hintedReq.Header.Set(service.RouterHeader, "direct-v1")
	hinted := httptest.NewRecorder()
	ServeHTTP(hinted, hintedReq)
	if hinted.Code != http.StatusOK || hinted.Body.String() != "direct" {
		t.Fatalf("hinted response = %d %q", hinted.Code, hinted.Body.String())
	}
}
