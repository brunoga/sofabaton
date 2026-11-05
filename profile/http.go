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
type HTTP struct {
	// Method is GET (the default), POST, PUT or DELETE.
	Method string `yaml:"method,omitempty"`
	// URL is plain http, to an IPv4 address: the hub reaches no names
	// and speaks no TLS here.
	URL         string   `yaml:"url"`
	Headers     []string `yaml:"headers,omitempty"`
	ContentType string   `yaml:"content_type,omitempty"`
	Body        string   `yaml:"body,omitempty"`
}

// Request is the request the command sends.
func (h HTTP) Request() (record.HTTPRequest, error) {
	u, err := url.Parse(h.URL)
	if err != nil {
		return record.HTTPRequest{}, fmt.Errorf("profile: http url: %w", err)
	}
	if u.Scheme != "http" || u.User != nil || u.Fragment != "" {
		return record.HTTPRequest{}, fmt.Errorf("profile: http url %q: the hub sends plain http:// requests, without a user or a #fragment", h.URL)
	}
	ip, err := netip.ParseAddr(u.Hostname())
	if err != nil || !ip.Is4() {
		return record.HTTPRequest{}, fmt.Errorf("profile: http url %q: the hub needs an IPv4 address, not a name", h.URL)
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
