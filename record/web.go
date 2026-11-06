package record

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// WebRequest is the signal of a ClassWeb command: a whole URL, http or
// https, with a method, headers and a body, all the hub's to send. Our X2
// sent them with its own User-Agent ("ESP32 HTTP Client/1.0"), Host and
// Content-Length, the headers given, and a Content-Type of
// application/x-www-form-urlencoded when none was; over TLS it took a
// self-signed certificate (2026-11-06). The layout, big-endian:
//
//	u16 length of it all | u16 URL length | URL | u8 method | u8 headers
//	per header: u8 name length | name | u16 value length | value
//	body
type WebRequest struct {
	// URL is http:// or https://, to a name or an address.
	URL string
	// Method is GET, POST or PUT.
	Method string
	// Headers are "Name: Value".
	Headers []string
	Body    string
}

var webMethods = []string{"GET", "POST", "PUT"}

// Data is the request as the hub stores it.
func (r WebRequest) Data() ([]byte, error) {
	u, err := url.Parse(r.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || strings.ContainsAny(r.URL, " \r\n") {
		return nil, fmt.Errorf("record: web request URL %q: http:// or https://", r.URL)
	}
	method := -1
	for i, m := range webMethods {
		if m == r.Method {
			method = i
		}
	}
	if method < 0 {
		return nil, fmt.Errorf("record: web request method %q: GET, POST or PUT", r.Method)
	}
	if len(r.Headers) > 0xFF {
		return nil, fmt.Errorf("record: web request of %d headers", len(r.Headers))
	}
	out := []byte{0, 0}
	out = binary.BigEndian.AppendUint16(out, uint16(len(r.URL)))
	out = append(out, r.URL...)
	out = append(out, byte(method), byte(len(r.Headers)))
	for _, h := range r.Headers {
		name, value, ok := strings.Cut(h, ":")
		value = strings.TrimPrefix(value, " ")
		if !ok || name == "" || len(name) > 0xFF || strings.ContainsAny(h, "\r\n") || strings.HasPrefix(value, " ") {
			return nil, fmt.Errorf("record: web request header %q: Name: Value", h)
		}
		out = append(out, byte(len(name)))
		out = append(out, name...)
		out = binary.BigEndian.AppendUint16(out, uint16(len(value)))
		out = append(out, value...)
	}
	out = append(out, r.Body...)
	if len(out) > 0xFFFF {
		return nil, fmt.Errorf("record: web request of %d bytes, the most is %d", len(out), 0xFFFF)
	}
	binary.BigEndian.PutUint16(out, uint16(len(out)))
	return out, nil
}

var errNotWeb = errors.New("record: not a web request as the hub stores it")

// ParseWebRequest decodes a ClassWeb command's signal. It fails unless
// Data gives the same bytes back.
func ParseWebRequest(data []byte) (WebRequest, error) {
	if len(data) < 6 || int(binary.BigEndian.Uint16(data)) != len(data) {
		return WebRequest{}, errNotWeb
	}
	p := data[2:]
	take := func(n int) ([]byte, bool) {
		if n > len(p) {
			return nil, false
		}
		b := p[:n]
		p = p[n:]
		return b, true
	}
	n, _ := take(2)
	u, ok := take(int(binary.BigEndian.Uint16(n)))
	if !ok || len(p) < 2 || int(p[0]) >= len(webMethods) {
		return WebRequest{}, errNotWeb
	}
	r := WebRequest{URL: string(u), Method: webMethods[p[0]]}
	count := int(p[1])
	p = p[2:]
	for range count {
		l, ok := take(1)
		if !ok {
			return WebRequest{}, errNotWeb
		}
		name, ok := take(int(l[0]))
		if !ok {
			return WebRequest{}, errNotWeb
		}
		vl, ok := take(2)
		if !ok {
			return WebRequest{}, errNotWeb
		}
		value, ok := take(int(binary.BigEndian.Uint16(vl)))
		if !ok {
			return WebRequest{}, errNotWeb
		}
		r.Headers = append(r.Headers, string(name)+": "+string(value))
	}
	r.Body = string(p)
	if again, err := r.Data(); err != nil || string(again) != string(data) {
		return WebRequest{}, errNotWeb
	}
	return r, nil
}
