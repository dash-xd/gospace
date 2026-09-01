package wasmhttp

import "net/http"

// Request is the version-one host/guest HTTP payload. Body is encoded as base64
// by encoding/json because it is a []byte.
type Request struct {
	Method string              `json:"method"`
	URL    string              `json:"url"`
	Host   string              `json:"host,omitempty"`
	Header map[string][]string `json:"header,omitempty"`
	Body   []byte              `json:"body,omitempty"`
}

type Response struct {
	Status int                 `json:"status"`
	Header map[string][]string `json:"header,omitempty"`
	Body   []byte              `json:"body,omitempty"`
}

func CloneHeader(src http.Header) map[string][]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string][]string, len(src))
	for key, values := range src {
		dst[key] = append([]string(nil), values...)
	}
	return dst
}
