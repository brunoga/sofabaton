package lifx

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

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

func TestTargets(t *testing.T) {
	hc, asked := to(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer TOKEN" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`[{"id":"d073d5","label":"Desk","group":{"id":"g1","name":"Office"}},{"id":"d073d6","label":"Shelf","group":{"id":"g1","name":"Office"}},{"id":"d073d7","label":"Bed","group":{"id":"g2","name":"Bedroom"}}]`))
	})
	groups, lights, err := Targets(context.Background(), "TOKEN", hc)
	if err != nil || len(groups) != 2 || groups[0].Name != "Bedroom" || len(lights) != 3 || lights[0] != (Target{"id:d073d7", "Bed"}) {
		t.Fatalf("%+v %+v %v", groups, lights, err)
	}
	if (*asked)[0] != "GET https://api.lifx.com/v1/lights/all" {
		t.Errorf("asked %v", *asked)
	}
	if _, _, err := Targets(context.Background(), "bad", hc); !errors.Is(err, ErrToken) {
		t.Errorf("bad token: %v", err)
	}

	d := Device("LIFX", "TOKEN", []Target{All, groups[1]}, Options{OnOff: true, Brightness: []int{10, 100}})
	if err := d.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(d.Commands) != 8 || d.Commands[0].Name != "All lights on" || d.Commands[7].Name != "Office 100%" {
		t.Fatalf("%+v", d.Commands)
	}
	for i, want := range map[int][2]string{0: {"https://api.lifx.com/v1/lights/all/state", `{"power":"on"}`}, 2: {"https://api.lifx.com/v1/lights/all/state", `{"power":"on","brightness":0.1}`}, 7: {"https://api.lifx.com/v1/lights/group_id:g1/state", `{"power":"on","brightness":1}`}} {
		sig, err := d.Commands[i].Signal(d)
		if err != nil {
			t.Fatal(err)
		}
		r, err := record.ParseWebRequest(sig)
		if err != nil || r.URL != want[0] || r.Body != want[1] || r.Method != "PUT" || r.Headers[1] != "Authorization: Bearer TOKEN" {
			t.Errorf("%d: %+v, %v", i, r, err)
		}
	}
}
