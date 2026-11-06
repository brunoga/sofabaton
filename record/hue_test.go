package record

import (
	"bytes"
	"net/netip"
	"strings"
	"testing"
)

// Commands as the app wrote them to our X2 (2026-11-06), with another
// key; the hub sent them to the bridge.
func TestHueRequest(t *testing.T) {
	const key = "0123456789abcdefghijABCDEFGHIJ0123456789"
	for _, c := range []struct {
		r    HueRequest
		want string
	}{
		{HueRequest{Path: "api/" + key + "/groups/3/action", Body: "{\n\"on\": true\n}", Length: 16}, "\x3c\x00\x21api/" + key + "/groups/3/actionContent-Length:16\n\n{\n\"on\": true\n}"},
		{HueRequest{Path: "api/" + key + "/groups/81/action", Body: "{\n\"on\": false\n}", Length: 17}, "\x3d\x00\x22api/" + key + "/groups/81/actionContent-Length:17\n\n{\n\"on\": false\n}"},
		{HueRequest{Path: "api/" + key + "/lights/27/state", Body: "{\n\"bri\": 203\n}", Length: 17}, "\x3c\x00\x21api/" + key + "/lights/27/stateContent-Length:17\n\n{\n\"bri\": 203\n}"},
	} {
		got, err := c.r.Data()
		if err != nil || !bytes.Equal(got, []byte(c.want)) {
			t.Errorf("%+v: %q, %v\nwant %q", c.r, got, err, c.want)
		}
		back, err := ParseHueRequest(got)
		if err != nil || back != c.r {
			t.Errorf("%q came back %+v, %v", got, back, err)
		}
	}
	// Ours: the body's length.
	if got, _ := (HueRequest{Path: "api/k/lights/1/state", Body: `{"on":true}`}).Data(); !strings.HasSuffix(string(got), "Content-Length:11\n\n{\"on\":true}") {
		t.Errorf("ours: %q", got)
	}
	for _, bad := range []HueRequest{{Path: ""}, {Path: "/api/x"}, {Path: "api/x y"}, {Path: "api/x", Body: string(make([]byte, 250))}} {
		if _, err := bad.Data(); err == nil {
			t.Errorf("%+v taken", bad)
		}
	}
	for _, odd := range []string{"", "\x05\x00\x00api", "\x03\x00\x02apixx", "\x03\x00\x05apix\n\nyz"} {
		if _, err := ParseHueRequest([]byte(odd)); err == nil {
			t.Errorf("%q taken", odd)
		}
	}

	// The tail the app wrote.
	app := []byte{252, 85, 192, 168, 1, 183, 252, 0, 0, 252, 2, 0, 0, 0, 252, 0, 252, 0, 252, 0, 0, 7, 1, 255}
	got := make([]byte, 60)
	if err := NewHueDeviceTail(netip.MustParseAddr("192.168.1.183")).Put(got); err != nil || !bytes.Equal(got[:len(app)], app) {
		t.Errorf("tail % x, %v\nthe app's % x", got[:len(app)], err, app)
	}
}
