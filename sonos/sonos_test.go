package sonos

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"testing"

	"github.com/brunoga/sofabaton/profile"
	"github.com/brunoga/sofabaton/record"
)

// to sends every request to the server, recording where it was for.
func to(t *testing.T, h http.HandlerFunc) (*http.Client, *[]string) {
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	var asked []string
	u, _ := url.Parse(srv.URL)
	return &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		asked = append(asked, r.Method+" "+r.URL.String())
		r.URL.Scheme, r.URL.Host = "http", u.Host
		return http.DefaultTransport.RoundTrip(r)
	})}, &asked
}

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSpeaker(t *testing.T) {
	var action, body string
	hc, asked := to(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/xml/device_description.xml":
			_, _ = w.Write([]byte(`<root xmlns="urn:schemas-upnp-org:device-1-0"><device><roomName>Kitchen</roomName><modelName>Sonos One</modelName></device></root>`))
		case "/MediaRenderer/RenderingControl/Control":
			action = r.Header.Get("SOAPACTION")
			b, _ := io.ReadAll(r.Body)
			body = string(b)
			_, _ = w.Write([]byte(`<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body><u:GetVolumeResponse xmlns:u="urn:schemas-upnp-org:service:RenderingControl:1"><CurrentVolume>23</CurrentVolume></u:GetVolumeResponse></s:Body></s:Envelope>`))
		default:
			http.NotFound(w, r)
		}
	})
	addr := netip.MustParseAddr("192.168.1.60")
	d, err := Describe(context.Background(), addr, hc)
	if err != nil || d.Room != "Kitchen" || d.Model != "Sonos One" {
		t.Fatalf("%+v, %v", d, err)
	}
	v, err := Volume(context.Background(), addr, hc)
	if err != nil || v != 23 || action != `"urn:schemas-upnp-org:service:RenderingControl:1#GetVolume"` || !strings.Contains(body, "<u:GetVolume ") {
		t.Fatalf("%d, %v; %s %s", v, err, action, body)
	}
	if (*asked)[0] != "GET http://192.168.1.60:1400/xml/device_description.xml" {
		t.Errorf("asked %v", *asked)
	}

	dev := Device("Kitchen", addr, v)
	if err := dev.Validate(); err != nil {
		t.Fatal(err)
	}
	var y bytes.Buffer
	if err := dev.Encode(&y); err != nil {
		t.Fatal(err)
	}
	back, err := profile.Load(&y)
	if err != nil || len(back.Commands) != 20 {
		t.Fatalf("%v, %d", err, len(back.Commands))
	}
	for _, c := range back.Commands {
		sig, err := c.Signal(back)
		if err != nil {
			t.Fatal(err)
		}
		r, err := record.ParseSonosRequest(sig)
		if err != nil {
			t.Fatalf("%s: %v", c.Name, err)
		}
		// Each action's envelope is of that action, its length its own.
		if _, act, _ := strings.Cut(r.Action, "#"); r.Action != "" && (r.Length != 0 || !strings.Contains(r.Body, "<u:"+act+" ")) {
			t.Errorf("%s: action %q, length %d, body %s", c.Name, r.Action, r.Length, r.Body)
		}
	}
	if k := back.Buttons["Vol+"]; k.Short != "Volume Up" || back.Commands[18].Sonos.Body != "23" {
		t.Errorf("Vol+ %+v, %+v", k, back.Commands[18].Sonos)
	}
}
