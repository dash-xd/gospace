package wasmguest

import (
	"net/http"
	"testing"
	"unsafe"

	"github.com/dash-xd/gospace/wasmhttp"
)

func TestHandleAdaptsServeMux(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /users/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Router", "mux")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(r.PathValue("id")))
	})
	input = wasmhttp.EncodeRequest(wasmhttp.Request{Method:http.MethodGet,URL:"/users/42?source=test",Host:"example.test",Header:http.Header{"X-Test":{"1"}}})
	ptr:=Handle(mux); size:=ResponseLen(); if ptr==nil||size==0{t.Fatal("empty encoded response")}; if ptr!=unsafe.Pointer(&output[0]){t.Fatal("response pointer does not reference output buffer")}
	response,err:=wasmhttp.DecodeResponse(output[:size]); if err!=nil{t.Fatal(err)}
	if response.Status!=http.StatusCreated{t.Fatalf("status = %d, want %d",response.Status,http.StatusCreated)}
	if values:=response.Header["X-Router"];len(values)!=1||values[0]!="mux"{t.Fatalf("X-Router = %v, want [mux]",values)}
	if got:=string(response.Body);got!="42"{t.Fatalf("body = %q, want 42",got)}
}
