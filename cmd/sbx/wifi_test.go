package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"testing"

	"github.com/brunoga/sofabaton/profile"
	"github.com/brunoga/sofabaton/roku"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// fakeNet has the Wi-Fi commands' requests, wherever they are for, served
// by h.
func fakeNet(t *testing.T, h http.HandlerFunc) {
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	prev := wifiHTTP
	wifiHTTP = &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		r.URL.Scheme, r.URL.Host = "http", u.Host
		return http.DefaultTransport.RoundTrip(r)
	})}
	t.Cleanup(func() { wifiHTTP = prev })
}

func TestWiFiCommands(t *testing.T) {
	fakeNet(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/query/apps":
			_, _ = w.Write([]byte(`<apps><app id="12">Netflix</app></apps>`))
		case r.URL.Path == "/query/device-info":
			_, _ = w.Write([]byte(`<device-info><user-device-name>Den</user-device-name></device-info>`))
		case r.URL.Path == "/xml/device_description.xml":
			_, _ = w.Write([]byte(`<root><device><roomName>Kitchen</roomName></device></root>`))
		case r.URL.Path == "/MediaRenderer/RenderingControl/Control":
			_, _ = w.Write([]byte(`<s:Envelope><s:Body><u:GetVolumeResponse><CurrentVolume>17</CurrentVolume></u:GetVolumeResponse></s:Body></s:Envelope>`))
		case r.URL.Path == "/v1/devices" && r.Header.Get("Govee-API-Key") == "GK":
			_, _ = w.Write([]byte(`{"code":200,"message":"Success","data":{"devices":[{"device":"AA","model":"H1","deviceName":"Lamp","controllable":true},{"device":"BB","model":"H1","deviceName":"Strip","controllable":true}]}}`))
		case r.URL.Path == "/v1/lights/all" && r.Header.Get("Authorization") == "Bearer LT":
			_, _ = w.Write([]byte(`[{"id":"1","label":"Desk","group":{"id":"g","name":"Office"}}]`))
		default:
			http.NotFound(w, r)
		}
	})
	sbx := func(args ...string) (*profile.Device, error) {
		var out, errs bytes.Buffer
		if err := run(context.Background(), strings.NewReader(""), args, &out, &errs); err != nil {
			return nil, err
		}
		return profile.Load(&out)
	}
	d, err := sbx("roku", "-address", "192.168.1.50")
	if err != nil || d.Name != "Den" || d.Roku.Address != "192.168.1.50" || d.Commands[len(d.Commands)-1].Roku != "launch/12" {
		t.Fatalf("roku: %+v, %v", d, err)
	}
	d, err = sbx("sonos", "-address", "192.168.1.60", "-name", "Cucina")
	if err != nil || d.Name != "Cucina" || d.Sonos.Address != "192.168.1.60" || d.Commands[len(d.Commands)-1].Sonos.Body != "17" {
		t.Fatalf("sonos: %+v, %v", d, err)
	}
	d, err = sbx("govee", "-key", "GK", "-only", "lamp", "-brightness", "50")
	if err != nil || len(d.Commands) != 3 || d.Commands[2].Name != "Lamp 50%" || !d.Commands[0].HTTP.Newer() {
		t.Fatalf("govee: %+v, %v", d, err)
	}
	if _, err := sbx("govee", "-key", "bad"); err == nil {
		t.Error("govee: a bad key taken")
	}
	d, err = sbx("lifx", "-token", "LT", "-brightness", "")
	if err != nil || len(d.Commands) != 6 || d.Commands[0].Name != "All lights on" || d.Commands[4].Name != "Desk on" {
		t.Fatalf("lifx: %+v, %v", d, err)
	}

	// Found on the network: the one there, or a choice.
	prev := findRokus
	t.Cleanup(func() { findRokus = prev })
	findRokus = func(context.Context) ([]roku.Player, error) {
		return []roku.Player{{Addr: netip.MustParseAddr("192.168.1.51"), Name: "Bedroom"}}, nil
	}
	if d, err = sbx("roku", "-no-apps"); err != nil || d.Name != "Bedroom" || d.Roku.Address != "192.168.1.51" {
		t.Fatalf("found roku: %+v, %v", d, err)
	}
	findRokus = func(context.Context) ([]roku.Player, error) {
		return []roku.Player{{Addr: netip.MustParseAddr("192.168.1.51")}, {Addr: netip.MustParseAddr("192.168.1.52")}}, nil
	}
	if _, err := sbx("roku"); err == nil || !strings.Contains(err.Error(), "-address") {
		t.Errorf("two rokus: %v", err)
	}
}
