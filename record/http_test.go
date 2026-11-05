package record

import (
	"net/netip"
	"reflect"
	"testing"
)

// Our X2's copy of a command upstream's create writes (2026-11-04).
const hubHTTP = "\xc0\xa8\x01W\x1f|\x00ePOST /zz/1/hello HTTP/1.1\r\nHost:192.168.1.87:8060\r\nContent-Type:application/x-www-form-urlencoded\r\n\r\n"

func TestHTTPRequest(t *testing.T) {
	r, err := ParseHTTPRequest([]byte(hubHTTP))
	if err != nil {
		t.Fatal(err)
	}
	want := HTTPRequest{Addr: netip.MustParseAddrPort("192.168.1.87:8060"), Method: "POST", Path: "/zz/1/hello", ContentType: "application/x-www-form-urlencoded"}
	if !reflect.DeepEqual(r, want) {
		t.Fatalf("parsed %+v", r)
	}

	for _, r := range []HTTPRequest{
		{Addr: netip.MustParseAddrPort("10.0.0.2:80"), Method: "GET", Path: "/status?x=1"},
		{Addr: netip.MustParseAddrPort("10.0.0.2:8123"), Method: "POST", Path: "/api/webhook/a", Headers: []string{"Authorization: Bearer t", "X-A: b"}, ContentType: "application/json", Body: "{\"on\":\r\n\r\ntrue}"},
		{Addr: netip.MustParseAddrPort("10.0.0.2:1"), Method: "PUT", Path: "/", Body: "x"},
	} {
		data, err := r.Data()
		if err != nil {
			t.Fatal(err)
		}
		back, err := ParseHTTPRequest(data)
		if err != nil || !reflect.DeepEqual(back, r) {
			t.Errorf("%+v came back %+v, %v", r, back, err)
		}
	}

	for _, bad := range []HTTPRequest{
		{Addr: netip.MustParseAddrPort("[::1]:80"), Method: "GET", Path: "/"},
		{Addr: netip.MustParseAddrPort("10.0.0.2:80"), Method: "GET", Path: "no-slash"},
		{Addr: netip.MustParseAddrPort("10.0.0.2:80"), Method: "GET", Path: "/", Headers: []string{"Host: x"}},
		{Addr: netip.MustParseAddrPort("10.0.0.2:80"), Method: "GET", Path: "/", Headers: []string{"A: b\r\nC: d"}},
	} {
		if _, err := bad.Data(); err == nil {
			t.Errorf("%+v taken", bad)
		}
	}
	// Bytes Data would not write are not taken as a request.
	for _, odd := range []string{hubHTTP + "\x00", hubHTTP[:20], "\x00\x00\x00\x00\x00\x50\x00\x02\r\n"} {
		if _, err := ParseHTTPRequest([]byte(odd)); err == nil {
			t.Errorf("%q taken", odd)
		}
	}
}

// Our X2 keeps a network device's tail as upstream's create leaves it,
// without the address's bytes (2026-11-04).
func TestCompactTail(t *testing.T) {
	stored := append([]byte{0xFC, 0, 1, 0xFC, 1, 1, 1, 0, 0xFC, 1, 0xFC, 1}, make([]byte, 48)...)
	got, err := ParseDeviceTail(stored)
	if err != nil {
		t.Fatal(err)
	}
	if got != NewIPDeviceTail() {
		t.Fatalf("parsed %+v", got)
	}
	back := make([]byte, len(stored))
	back[30] = 7 // kept
	if err := got.Put(back); err != nil {
		t.Fatal(err)
	}
	stored[30] = 7
	if !reflect.DeepEqual(back, stored) {
		t.Fatalf("put % x", back)
	}
	got.Color = 2
	if err := got.Put(back); err == nil {
		t.Error("a color put in a compact tail")
	}
}
