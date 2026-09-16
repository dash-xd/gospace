package wasmhttp

import (
	"bytes"
	"net/http"
	"testing"
)

func TestRequestRoundTripPreservesRawBodyAndRepeatedHeaders(t *testing.T) {
	body := []byte{0, 1, 2, 0xff, '\n'}
	encoded := EncodeRequest(Request{Method:"POST",URL:"/x?q=1",Host:"example.test",Header:http.Header{"X-Test":{"a","b"}},Body:body})
	if bytes.Contains(encoded, []byte("AAEC")) { t.Fatal("body appears to be base64 encoded") }
	got,err:=DecodeRequest(encoded); if err!=nil{t.Fatal(err)}
	if got.Method!="POST"||got.URL!="/x?q=1"||got.Host!="example.test"{t.Fatalf("request metadata changed: %#v",got)}
	if values:=got.Header["X-Test"];len(values)!=2||values[0]!="a"||values[1]!="b"{t.Fatalf("headers = %v",values)}
	if !bytes.Equal(got.Body,body){t.Fatalf("body = %v, want %v",got.Body,body)}
}

func TestResponseRoundTrip(t *testing.T) {
	encoded:=EncodeResponse(Response{Status:http.StatusCreated,Header:http.Header{"Content-Type":{"application/octet-stream"}},Body:[]byte{0,0xff}})
	got,err:=DecodeResponse(encoded);if err!=nil{t.Fatal(err)}
	if got.Status!=http.StatusCreated{t.Fatalf("status = %d",got.Status)}
	if !bytes.Equal(got.Body,[]byte{0,0xff}){t.Fatalf("body = %v",got.Body)}
}

func TestDecodeRejectsTruncatedMessage(t *testing.T) {
	encoded:=EncodeRequest(Request{Method:"GET",URL:"/"})
	if _,err:=DecodeRequest(encoded[:len(encoded)-1]);err==nil{t.Fatal("expected truncated message error")}
}
