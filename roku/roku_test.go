package roku

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
)

// to sends every request to the server, recording where it was for.
func to(t *testing.T, h http.HandlerFunc) (*http.Client, *[]string) {
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	var asked []string
	u, _ := url.Parse(srv.URL)
	return &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		asked = append(asked, r.URL.String())
		r.URL.Host = u.Host
		return http.DefaultTransport.RoundTrip(r)
	})}, &asked
}

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestAppsAndDevice(t *testing.T) {
	hc, asked := to(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/query/apps":
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8" ?>
<apps>
	<app id="12" type="appl" version="5.1">Netflix</app>
	<app id="2213" type="appl" version="1.0">Roku Media Player</app>
	<app id="tvinput.hdmi1" type="tvin" version="1.0">HDMI 1</app>
	<app id="bad id" type="appl">Nope</app>
</apps>`))
		case "/query/device-info":
			_, _ = w.Write([]byte(`<device-info><serial-number>X1</serial-number><user-device-name>Den</user-device-name><model-name>Roku Ultra</model-name></device-info>`))
		default:
			http.NotFound(w, r)
		}
	})
	addr := netip.MustParseAddr("192.168.1.50")
	apps, err := Apps(context.Background(), addr, hc)
	if err != nil || len(apps) != 3 || apps[0] != (App{ID: "12", Name: "Netflix"}) || apps[2].ID != "tvinput.hdmi1" {
		t.Fatalf("%+v, %v", apps, err)
	}
	info, err := Info(context.Background(), addr, hc)
	if err != nil || info.Name() != "Den" || info.Serial != "X1" {
		t.Fatalf("%+v, %v", info, err)
	}
	if (*asked)[0] != "http://192.168.1.50:8060/query/apps" {
		t.Errorf("asked %v", *asked)
	}

	apps = append(apps, App{ID: "99", Name: "Home"}) // a name a key has
	d := Device("Den Roku", addr, apps)
	if err := d.Validate(); err != nil {
		t.Fatal(err)
	}
	var yaml bytes.Buffer
	if err := d.Encode(&yaml); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"roku:\n  address: 192.168.1.50", "name: Home\n    roku: keypress/Home", "name: Netflix\n    roku: launch/12", "name: Home 2\n    roku: launch/99", "OK: OK", "Vol+: Volume Up", `"on": Power On`} {
		if !strings.Contains(yaml.String(), want) {
			t.Errorf("no %q in\n%s", want, yaml.String())
		}
	}
	back, err := profile.Load(&yaml)
	if err != nil || len(back.Commands) != len(keys)+4 {
		t.Fatalf("%v, %d commands", err, len(back.Commands))
	}

	many := make([]App, 300)
	for i := range many {
		many[i] = App{ID: "1", Name: "A"}
	}
	if n := len(Device("x", addr, many).Commands); n != profile.MaxCommands {
		t.Errorf("%d commands", n)
	}
}
