package wasmguest

import (
	"bytes"
	"net/http"
	"unsafe"

	"github.com/dash-xd/gospace/wasmhttp"
)

var ( input []byte; output []byte )

type responseWriter struct { header http.Header; body bytes.Buffer; status int }
func newResponseWriter()*responseWriter{return &responseWriter{header:make(http.Header)}}
func(w *responseWriter)Header()http.Header{return w.header}
func(w *responseWriter)WriteHeader(status int){if w.status!=0{return};w.status=status}
func(w *responseWriter)Write(p []byte)(int,error){if w.status==0{w.status=http.StatusOK};return w.body.Write(p)}

func Alloc(size uint32) unsafe.Pointer { input=make([]byte,size); if len(input)==0{return nil}; return unsafe.Pointer(&input[0]) }
func Handle(handler http.Handler) unsafe.Pointer {
	if handler==nil{return encodeError(http.StatusInternalServerError,"nil router")}
	in,err:=wasmhttp.DecodeRequest(input); if err!=nil{return encodeError(http.StatusBadRequest,"invalid host request")}
	req,err:=http.NewRequest(in.Method,in.URL,bytes.NewReader(in.Body)); if err!=nil{return encodeError(http.StatusBadRequest,"invalid request URL")}; req.Host=in.Host; req.Header=in.Header
	rw:=newResponseWriter(); handler.ServeHTTP(rw,req); if rw.status==0{rw.status=http.StatusOK}
	return encode(wasmhttp.Response{Status:rw.status,Header:rw.header,Body:rw.body.Bytes()})
}
func ResponseLen()uint32{return uint32(len(output))}
func encodeError(status int,message string)unsafe.Pointer{return encode(wasmhttp.Response{Status:status,Header:http.Header{"Content-Type":{"text/plain; charset=utf-8"}},Body:[]byte(message+"\n")})}
func encode(response wasmhttp.Response)unsafe.Pointer{output=wasmhttp.EncodeResponse(response);if len(output)==0{return nil};return unsafe.Pointer(&output[0])}
