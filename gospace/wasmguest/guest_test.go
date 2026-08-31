package wasmguest

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/dash-xd/gospace/wasmhttp"
)

func TestHandleAdaptsServeMux(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /users/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Router", "mux")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(r.PathValue("id")))
	})

	encoded, err := json.Marshal(wasmhttp.Request{
		Method: http.MethodGet,
		URL:    "/users/42?source=test",
		Host:   "example.test",
		Header: map[string][]string{"X-Test": {"1"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	input = encoded

	packed := Handle(mux)
	ptr, size := uint32(packed>>32), uint32(packed)
	if size == 0 {
		t.Fatal("empty encoded response")
	}

	base := uint32(uintptr(unsafe.Pointer(&output[0])))
	if ptr != base {
		t.Fatalf("response pointer = %d, want %d", ptr, base)
	}

	var response wasmhttp.Response
	if err := json.Unmarshal(output[:size], &response); err != nil {
		t.Fatal(err)
	}
	if response.Status != http.StatusCreated {
		t.Fatalf("status = %d, want %d", response.Status, http.StatusCreated)
	}
	if got := response.Header.Get("X-Router"); got != "mux" {
		t.Fatalf("X-Router = %q, want mux", got)
	}
	if got := string(response.Body); got != "42" {
		t.Fatalf("body = %q, want 42", got)
	}
}
