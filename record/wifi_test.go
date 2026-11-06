package record

import (
	"bytes"
	"encoding/hex"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// Requests our X2 took and sent (2026-11-06), from devices pointed at a
// listener: a Roku's, a Sonos's and a web request's.
func TestRokuRequest(t *testing.T) {
	r := RokuRequest{Path: "keypress/Home"}
	got, err := r.Data()
	if err != nil || string(got) != "\x0dkeypress/Home" {
		t.Fatalf("%q, %v", got, err)
	}
	if back, err := ParseRokuRequest(got); err != nil || back != r {
		t.Errorf("came back %+v, %v", back, err)
	}
	for _, bad := range []string{"", "/keypress/Home", "keypress/A B"} {
		if _, err := (RokuRequest{Path: bad}).Data(); err == nil {
			t.Errorf("path %q taken", bad)
		}
	}
	for _, bad := range []string{"", "\x05keypress/Home", "\x00"} {
		if _, err := ParseRokuRequest([]byte(bad)); err == nil {
			t.Errorf("%q parsed", bad)
		}
	}
}

func TestSonosRequest(t *testing.T) {
	// The app's Play, Content-Length and space as it writes them.
	const play = `<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"><s:Body><u:Play xmlns:u="urn:schemas-upnp-org:service:AVTransport:1"><InstanceID>0</InstanceID><Speed>1</Speed></u:Play></s:Body></s:Envelope>` + "\n"
	app := SonosRequest{Path: "MediaRenderer/AVTransport/Control", Action: "urn:schemas-upnp-org:service:AVTransport:1#Play", Body: play, Length: 266, Space: true}
	ours := SonosRequest{Path: "MediaRenderer/AVTransport/Control", Action: "urn:schemas-upnp-org:service:AVTransport:1#Play", Body: play}
	step := SonosRequest{Path: "MediaRenderer/VolumeUp/Control", Body: "30"}
	for _, r := range []SonosRequest{app, ours, step} {
		got, err := r.Data()
		if err != nil {
			t.Fatal(err)
		}
		if back, err := ParseSonosRequest(got); err != nil || back != r {
			t.Errorf("%q came back %+v, %v", got, back, err)
		}
	}
	got, _ := app.Data()
	rest := "SOAPACTION: \"urn:schemas-upnp-org:service:AVTransport:1#Play\" \nContent-Length:266\n\n" + play
	want := append([]byte{33, byte(len(rest) >> 8), byte(len(rest))}, "MediaRenderer/AVTransport/Control"+rest...)
	if !bytes.Equal(got, want) {
		t.Errorf("%q\nwant %q", got, want)
	}
	if got, _ := step.Data(); string(got) != "\x1e\x00\x02MediaRenderer/VolumeUp/Control30" {
		t.Errorf("step %q", got)
	}
	if got, _ := ours.Data(); !strings.Contains(string(got), "#Play\"\nContent-Length:"+strconv.Itoa(len(play))+"\n\n") {
		t.Errorf("ours %q", got)
	}
}

func TestWebRequest(t *testing.T) {
	// As written to our X2, which sent PUT /zz21 with these headers and
	// body, and the second over TLS.
	for _, c := range []struct {
		r    WebRequest
		data string
	}{
		{WebRequest{URL: "http://192.168.1.87:8099/zz21", Method: "PUT", Headers: []string{"Content-Type: application/json", "X-Zz: hello"}, Body: `{"power":"on"}`},
			"005c001d687474703a2f2f3139322e3136382e312e38373a383039392f7a7a323102020c436f6e74656e742d5479706500106170706c69636174696f6e2f6a736f6e04582d5a7a000568656c6c6f7b22706f776572223a226f6e227d"},
		{WebRequest{URL: "https://192.168.1.87:8443/zz21s", Method: "POST", Headers: []string{"Authorization: Bearer zz"}, Body: `{"a":1}`},
			"0045001f68747470733a2f2f3139322e3136382e312e38373a383434332f7a7a32317301010d417574686f72697a6174696f6e0009426561726572207a7a7b2261223a317d"},
		{WebRequest{URL: "https://api.lifx.com/v1/lights/all/state", Method: "GET"}, ""},
	} {
		got, err := c.r.Data()
		if err != nil {
			t.Fatal(err)
		}
		if c.data != "" && hex.EncodeToString(got) != c.data {
			t.Errorf("%+v:\n%x\nwant %s", c.r, got, c.data)
		}
		if back, err := ParseWebRequest(got); err != nil || !reflect.DeepEqual(back, c.r) {
			t.Errorf("%x came back %+v, %v", got, back, err)
		}
	}
	for _, bad := range []WebRequest{
		{URL: "ftp://x/y", Method: "GET"},
		{URL: "http://x/y", Method: "DELETE"},
		{URL: "http://x/y", Method: "GET", Headers: []string{"no colon"}},
		{URL: "http://x/y", Method: "GET", Headers: []string{"A:  two spaces"}},
	} {
		if _, err := bad.Data(); err == nil {
			t.Errorf("%+v taken", bad)
		}
	}
	if _, err := ParseWebRequest([]byte{0, 9, 0, 1}); err == nil {
		t.Error("short data parsed")
	}
}
