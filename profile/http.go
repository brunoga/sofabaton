package profile

import (
	"fmt"
	"net/netip"
	"net/url"
	"strconv"
	"strings"

	"github.com/brunoga/sofabaton/record"
)

// HTTP is a network command: the HTTP request the hub sends when it runs
// (a device of them is a generic network device, class wifi_ip).
//
//   - name: Lights on
//     http:
//     method: POST
//     url: http://192.168.1.50:8123/api/webhook/lights
//     headers: ["Authorization: Bearer abc"]
//     content_type: application/json
//     body: '{"on": true}'
//
// A request to an IPv4 address over plain http is stored in the hub's
// older form (record.HTTPRequest), which every hub takes; one over https,
// or to a name, in the X2's newer one (record.WebRequest, as the app
// writes them all on the X2), and on the X2 only.
type HTTP struct {
	// Method is GET (the default), POST, PUT or DELETE (DELETE in the
	// older form only).
	Method string `yaml:"method,omitempty"`
	// URL is http or https, to an address or a name.
	URL         string   `yaml:"url"`
	Headers     []string `yaml:"headers,omitempty"`
	ContentType string   `yaml:"content_type,omitempty"`
	Body        string   `yaml:"body,omitempty"`

	// newer is set on a request read from the hub in the newer form, so
	// it is written back in it.
	newer bool
}

// Newer reports whether the request is stored in the X2's newer form:
// it is https, or to a name, or was read from the hub in it.
func (h HTTP) Newer() bool {
	if h.newer {
		return true
	}
	u, err := url.Parse(h.URL)
	if err != nil {
		return false
	}
	ip, err := netip.ParseAddr(u.Hostname())
	return u.Scheme == "https" || err != nil || !ip.Is4()
}

// WebRequest is the request the command sends, in the newer form.
func (h HTTP) WebRequest() (record.WebRequest, error) {
	method := strings.ToUpper(h.Method)
	if method == "" {
		method = "GET"
	}
	r := record.WebRequest{URL: h.URL, Method: method, Body: h.Body}
	if h.ContentType != "" {
		r.Headers = append(r.Headers, "Content-Type: "+h.ContentType)
	}
	r.Headers = append(r.Headers, h.Headers...)
	if _, err := r.Data(); err != nil {
		return record.WebRequest{}, fmt.Errorf("profile: http: %w", err)
	}
	return r, nil
}

// webOf is a request of the newer form as a profile writes it: a first
// Content-Type header is its content_type.
func webOf(r record.WebRequest) *HTTP {
	h := &HTTP{URL: r.URL, Body: r.Body, newer: true}
	if r.Method != "GET" {
		h.Method = r.Method
	}
	headers := r.Headers
	if len(headers) > 0 {
		if ct, ok := strings.CutPrefix(headers[0], "Content-Type: "); ok {
			h.ContentType, headers = ct, headers[1:]
		}
	}
	h.Headers = headers
	return h
}

// data is the request as the hub stores it, in its form.
func (h HTTP) data() ([]byte, error) {
	if h.Newer() {
		r, err := h.WebRequest()
		if err != nil {
			return nil, err
		}
		return r.Data()
	}
	r, err := h.Request()
	if err != nil {
		return nil, err
	}
	return r.Data()
}

// Request is the request the command sends, in the older form.
func (h HTTP) Request() (record.HTTPRequest, error) {
	u, err := url.Parse(h.URL)
	if err != nil {
		return record.HTTPRequest{}, fmt.Errorf("profile: http url: %w", err)
	}
	if u.Scheme != "http" || u.User != nil || u.Fragment != "" {
		return record.HTTPRequest{}, fmt.Errorf("profile: http url %q: this form is plain http://, without a user or a #fragment", h.URL)
	}
	ip, err := netip.ParseAddr(u.Hostname())
	if err != nil || !ip.Is4() {
		return record.HTTPRequest{}, fmt.Errorf("profile: http url %q: this form needs an IPv4 address, not a name", h.URL)
	}
	port := uint64(80)
	if p := u.Port(); p != "" {
		if port, err = strconv.ParseUint(p, 10, 16); err != nil || port == 0 {
			return record.HTTPRequest{}, fmt.Errorf("profile: http url %q: port %q", h.URL, p)
		}
	}
	method := strings.ToUpper(h.Method)
	switch method {
	case "":
		method = "GET"
	case "GET", "POST", "PUT", "DELETE":
	default:
		return record.HTTPRequest{}, fmt.Errorf("profile: http method %q: GET, POST, PUT or DELETE", h.Method)
	}
	return record.HTTPRequest{
		Addr:   netip.AddrPortFrom(ip, uint16(port)),
		Method: method, Path: u.RequestURI(),
		Headers: h.Headers, ContentType: h.ContentType, Body: h.Body,
	}, nil
}

// httpOf is a request as a profile writes it.
func httpOf(r record.HTTPRequest) *HTTP {
	host := r.Addr.Addr().String()
	if r.Addr.Port() != 80 {
		host = r.Addr.String()
	}
	h := &HTTP{URL: "http://" + host + r.Path, Headers: r.Headers, ContentType: r.ContentType, Body: r.Body}
	if r.Method != "GET" {
		h.Method = r.Method
	}
	return h
}

// network reports whether the device's commands are HTTP requests: a new
// device of them is a generic network device.
func (d *Device) network() bool {
	for _, c := range d.Commands {
		if c.HTTP != nil {
			return true
		}
	}
	return false
}
