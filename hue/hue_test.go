package hue

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

	"github.com/brunoga/sofabaton/profile"
)

// bridge serves the API at a fake address, its link button pressed once
// pressed is set.
func bridge(t *testing.T, pressed *bool) *http.Client {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(b), `"devicetype":"armrest#test"`) {
			t.Errorf("link body %s", b)
		}
		if !*pressed {
			_, _ = io.WriteString(w, `[{"error":{"type":101,"address":"","description":"link button not pressed"}}]`)
			return
		}
		_, _ = io.WriteString(w, `[{"success":{"username":"KEY"}}]`)
	})
	mux.HandleFunc("GET /api/KEY/groups", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"81":{"name":"Cinema","type":"Room"},"3":{"name":"Backyard","type":"Room"},"200":{"name":"TV","type":"Entertainment"}}`)
	})
	mux.HandleFunc("GET /api/KEY/lights", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"27":{"name":"Pool Table"},"4":{"name":"Poster The Godfather"}}`)
	})
	mux.HandleFunc("GET /api/OLD/lights", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `[{"error":{"type":1,"address":"/lights","description":"unauthorized user"}}]`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &http.Client{Transport: &http.Transport{DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, srv.Listener.Addr().String())
	}}}
}

func TestBridge(t *testing.T) {
	ctx := context.Background()
	pressed := false
	hc := bridge(t, &pressed)
	addr := netip.MustParseAddr("192.168.1.183")
	if _, err := Link(ctx, addr, "armrest#test", hc); !errors.Is(err, ErrLinkButton) {
		t.Fatalf("before the button: %v", err)
	}
	pressed = true
	c, err := Link(ctx, addr, "armrest#test", hc)
	if err != nil || c.User != "KEY" {
		t.Fatalf("%+v %v", c, err)
	}
	rooms, err := c.Rooms(ctx)
	if err != nil || len(rooms) != 2 || rooms[0].Name != "Backyard" || rooms[1].ID != "81" || !rooms[0].Group {
		t.Fatalf("rooms %+v %v", rooms, err)
	}
	lights, err := c.Lights(ctx)
	if err != nil || len(lights) != 2 || lights[0].ID != "4" || lights[0].Group {
		t.Fatalf("lights %+v %v", lights, err)
	}
	if _, err := (&Client{Addr: addr, User: "OLD", HTTP: hc}).Lights(ctx); err == nil || !strings.Contains(err.Error(), "unauthorized") {
		t.Errorf("unknown user: %v", err)
	}

	// The commands as the app made them for our X2's bridge.
	cmds := Commands(append(rooms[1:], lights[1], lights[0]), Options{OnOff: true, Brightness: Brightness})
	var names []string
	for _, c := range cmds {
		names = append(names, c.Name)
	}
	want := "Cinema on|Cinema off|Cinema 10% Brightness|Cinema 50% Brightness|Cinema 80% Brightness|" +
		"Pool Table on|Pool Table off|Poster The Godfather on|Poster The Godfather off|" +
		"Pool Table 10% Brightness|Pool Table 50% Brightness|Pool Table 80% Brightness|" +
		"Poster The Godf 10% Brightness|Poster The Godf 50% Brightness|Poster The Godf 80% Brightness"
	if got := strings.Join(names, "|"); got != want {
		t.Errorf("names\n%s\nwant\n%s", got, want)
	}
	for i, w := range []profile.HueCommand{
		{Path: "groups/81/action", Body: "{\n\"on\": true\n}"},
		{Path: "groups/81/action", Body: "{\n\"on\": false\n}"},
		{Path: "groups/81/action", Body: "{\n\"bri\": 25\n}"},
		{Path: "groups/81/action", Body: "{\n\"bri\": 127\n}"},
		{Path: "groups/81/action", Body: "{\n\"bri\": 203\n}"},
		{Path: "lights/27/state", Body: "{\n\"on\": true\n}"},
	} {
		if *cmds[i].Hue != w {
			t.Errorf("command %d %+v, want %+v", i, *cmds[i].Hue, w)
		}
	}
	d := Device(c, cmds)
	if d.Name != "Hue Bridge (192.168.1.183)" || d.Validate() != nil {
		t.Errorf("device %+v: %v", d, d.Validate())
	}

	// Long names keep what the command does; names that come out the
	// same get a number.
	dup := Commands([]Target{{ID: "1", Name: "A very long light name that is cut"}, {ID: "2", Name: "A very long light name that is cut too"}}, Options{OnOff: true})
	var got []string
	for _, c := range dup {
		got = append(got, c.Name)
	}
	if strings.Join(got, "|") != "A very long light name that on|A very long light name tha off|A very long light name th on 2|A very long light name t off 2" {
		t.Errorf("%q", got)
	}
}
