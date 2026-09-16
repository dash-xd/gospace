package wasmhttp

import (
	"encoding/binary"
	"errors"
	"net/http"
)

const (
	requestMagic  = "GSR1"
	responseMagic = "GSS1"
)

// Request is the host/guest HTTP payload. The wire encoding is deliberately
// binary and length-prefixed: bodies remain raw bytes and headers preserve
// repeated values without JSON/base64 or delimiter escaping.
type Request struct {
	Method string
	URL    string
	Host   string
	Header http.Header
	Body   []byte
}

type Response struct {
	Status int
	Header http.Header
	Body   []byte
}

func EncodeRequest(r Request) []byte {
	return encodeMessage(requestMagic, 0, r.Method, r.URL, r.Host, r.Header, r.Body)
}

func DecodeRequest(data []byte) (Request, error) {
	status, method, url, host, header, body, err := decodeMessage(data, requestMagic)
	if err != nil {
		return Request{}, err
	}
	if status != 0 {
		return Request{}, errors.New("invalid request status")
	}
	return Request{Method: method, URL: url, Host: host, Header: header, Body: body}, nil
}

func EncodeResponse(r Response) []byte {
	return encodeMessage(responseMagic, uint32(r.Status), "", "", "", r.Header, r.Body)
}

func DecodeResponse(data []byte) (Response, error) {
	status, method, url, host, header, body, err := decodeMessage(data, responseMagic)
	if err != nil {
		return Response{}, err
	}
	if method != "" || url != "" || host != "" {
		return Response{}, errors.New("invalid response fields")
	}
	return Response{Status: int(status), Header: header, Body: body}, nil
}

func encodeMessage(magic string, status uint32, method, url, host string, header http.Header, body []byte) []byte {
	headerCount := 0
	size := 4 + 4 + 5*4 + len(method) + len(url) + len(host) + len(body)
	for key, values := range header {
		for _, value := range values {
			headerCount++
			size += 8 + len(key) + len(value)
		}
	}
	buf := make([]byte, size)
	copy(buf, magic)
	off := 4
	put32 := func(v uint32) { binary.LittleEndian.PutUint32(buf[off:off+4], v); off += 4 }
	put32(status)
	put32(uint32(len(method)))
	put32(uint32(len(url)))
	put32(uint32(len(host)))
	put32(uint32(headerCount))
	put32(uint32(len(body)))
	put := func(s string) { copy(buf[off:], s); off += len(s) }
	put(method); put(url); put(host)
	for key, values := range header {
		for _, value := range values {
			put32(uint32(len(key))); put32(uint32(len(value))); put(key); put(value)
		}
	}
	copy(buf[off:], body)
	return buf
}

func decodeMessage(data []byte, magic string) (uint32, string, string, string, http.Header, []byte, error) {
	if len(data) < 28 || string(data[:4]) != magic {
		return 0, "", "", "", nil, nil, errors.New("invalid gospace WASM message")
	}
	off := 4
	read32 := func() (uint32, bool) {
		if off+4 > len(data) { return 0, false }
		v := binary.LittleEndian.Uint32(data[off:off+4]); off += 4; return v, true
	}
	status, _ := read32()
	methodLen, _ := read32(); urlLen, _ := read32(); hostLen, _ := read32(); headerCount, _ := read32(); bodyLen, _ := read32()
	readString := func(n uint32) (string, bool) {
		if uint64(off)+uint64(n) > uint64(len(data)) { return "", false }
		s := string(data[off:off+int(n)]); off += int(n); return s, true
	}
	method, ok := readString(methodLen); if !ok { return 0, "", "", "", nil, nil, errors.New("truncated method") }
	url, ok := readString(urlLen); if !ok { return 0, "", "", "", nil, nil, errors.New("truncated URL") }
	host, ok := readString(hostLen); if !ok { return 0, "", "", "", nil, nil, errors.New("truncated host") }
	header := make(http.Header)
	for i := uint32(0); i < headerCount; i++ {
		keyLen, ok := read32(); if !ok { return 0, "", "", "", nil, nil, errors.New("truncated header key length") }
		valueLen, ok := read32(); if !ok { return 0, "", "", "", nil, nil, errors.New("truncated header value length") }
		key, ok := readString(keyLen); if !ok { return 0, "", "", "", nil, nil, errors.New("truncated header key") }
		value, ok := readString(valueLen); if !ok { return 0, "", "", "", nil, nil, errors.New("truncated header value") }
		header[key] = append(header[key], value)
	}
	if uint64(off)+uint64(bodyLen) != uint64(len(data)) {
		return 0, "", "", "", nil, nil, errors.New("invalid body length")
	}
	return status, method, url, host, header, data[off:], nil
}
