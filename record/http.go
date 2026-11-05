package record

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net/netip"
	"strconv"
	"strings"
)

// HTTPRequest is the signal of a generic network command (ClassIP): an
// HTTP request the hub sends when the command runs. The hub stores the
// address, the port and the request's text, which it writes itself
// (upstream's blob_decoders.py, checked on our X2 2026-11-04):
//
//	c0 a8 01 57      address
//	1f 7c            port
//	00 65            text length
//	POST /zz HTTP/1.1\r\nHost:192.168.1.87:8060\r\n...
//
// The text is the request line, the Host line, the headers, a
// Content-Type line and, with a body, a Content-Length line, a blank
// line and the body; a blank line ends it.
type HTTPRequest struct {
	// Addr is where the request goes: an IPv4 address and a port.
	Addr netip.AddrPort
	// Method is GET, POST, PUT or DELETE; Path the request's target,
	// with any query.
	Method, Path string
	// Headers are header lines ("Authorization: Bearer x"), besides
	// Host, Content-Type and Content-Length.
	Headers     []string
	ContentType string
	Body        string
}

// maxHTTPText is the most text the length field holds.
const maxHTTPText = 0xFFFF

// Data is the request as the hub stores it.
func (r HTTPRequest) Data() ([]byte, error) {
	if !r.Addr.Addr().Is4() {
		return nil, fmt.Errorf("record: HTTP request to %v: the hub takes IPv4 addresses only", r.Addr.Addr())
	}
	if r.Method == "" || strings.ContainsAny(r.Method, " \r\n") {
		return nil, fmt.Errorf("record: HTTP method %q", r.Method)
	}
	if !strings.HasPrefix(r.Path, "/") || strings.ContainsAny(r.Path, " \r\n") {
		return nil, fmt.Errorf("record: HTTP path %q: it starts with / and has no spaces", r.Path)
	}
	for _, h := range append(r.Headers[:len(r.Headers):len(r.Headers)], r.ContentType) {
		if strings.ContainsAny(h, "\r\n") {
			return nil, fmt.Errorf("record: HTTP header %q spans lines", h)
		}
	}
	for _, h := range r.Headers {
		if !strings.Contains(h, ":") || specialHeader(h) {
			return nil, fmt.Errorf("record: HTTP header %q: a name and a value, and not Host, Content-Type or Content-Length", h)
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s HTTP/1.1\r\nHost:%v:%d\r\n", r.Method, r.Path, r.Addr.Addr(), r.Addr.Port())
	for _, h := range r.Headers {
		b.WriteString(h + "\r\n")
	}
	if r.ContentType != "" {
		b.WriteString("Content-Type:" + r.ContentType + "\r\n")
	}
	if r.Body != "" {
		fmt.Fprintf(&b, "Content-Length:%d\r\n\r\n%s", len(r.Body), r.Body)
	}
	b.WriteString("\r\n")
	text := b.String()
	if len(text) > maxHTTPText {
		return nil, fmt.Errorf("record: HTTP request of %d bytes, the most is %d", len(text), maxHTTPText)
	}
	ip := r.Addr.Addr().As4()
	out := append([]byte(nil), ip[:]...)
	out = binary.BigEndian.AppendUint16(out, r.Addr.Port())
	out = binary.BigEndian.AppendUint16(out, uint16(len(text)))
	return append(out, text...), nil
}

// specialHeader reports whether h is one of the lines Data writes itself.
func specialHeader(h string) bool {
	name, _, _ := strings.Cut(h, ":")
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "host", "content-type", "content-length":
		return true
	}
	return false
}

// errNotHTTP reports data that is not an HTTP request Data writes.
var errNotHTTP = errors.New("record: not an HTTP request as the hub stores it")

// ParseHTTPRequest decodes an HTTP command's signal. It fails unless Data
// gives the same bytes back, so a request it decodes can be edited and
// written without changing anything else.
func ParseHTTPRequest(data []byte) (HTTPRequest, error) {
	if len(data) < 8 {
		return HTTPRequest{}, errNotHTTP
	}
	n := int(binary.BigEndian.Uint16(data[6:]))
	if len(data) != 8+n {
		return HTTPRequest{}, errNotHTTP
	}
	r := HTTPRequest{Addr: netip.AddrPortFrom(netip.AddrFrom4([4]byte(data[:4])), binary.BigEndian.Uint16(data[4:]))}
	text := string(data[8:])
	head, body, hasBody := strings.Cut(text, "\r\n\r\n")
	lines := strings.Split(head, "\r\n")
	method, path, ok := strings.Cut(lines[0], " ")
	if !ok {
		return HTTPRequest{}, errNotHTTP
	}
	r.Method, r.Path = method, strings.TrimSuffix(path, " HTTP/1.1")
	for _, l := range lines[1:] {
		name, value, _ := strings.Cut(l, ":")
		switch strings.ToLower(name) {
		case "host":
		case "content-type":
			r.ContentType = value
		case "content-length":
			if _, err := strconv.Atoi(value); err != nil {
				return HTTPRequest{}, errNotHTTP
			}
			r.Body = strings.TrimSuffix(body, "\r\n")
		default:
			r.Headers = append(r.Headers, l)
		}
	}
	if !hasBody {
		return HTTPRequest{}, errNotHTTP
	}
	if again, err := r.Data(); err != nil || string(again) != string(data) {
		return HTTPRequest{}, errNotHTTP
	}
	return r, nil
}
