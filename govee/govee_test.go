package govee

import (
	"context"
	"encoding/json"
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

func TestLights(t *testing.T) {
	hc, asked := to(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Govee-API-Key") != "KEY" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"data":{"devices":[{"device":"AA:BB","model":"H6159","deviceName":"Lamp","controllable":true,"retrievable":true,"supportCmds":["turn","brightness"]},{"device":"CC","model":"H5074","deviceName":"Thermometer","controllable":false}]},"message":"Success","code":200}`))
	})
	ls, err := Lights(context.Background(), "KEY", hc)
	if err != nil || len(ls) != 1 || ls[0].Name != "Lamp" || ls[0].Model != "H6159" {
		t.Fatalf("%+v, %v", ls, err)
	}
	if (*asked)[0] != "GET https://developer-api.govee.com/v1/devices" {
		t.Errorf("asked %v", *asked)
	}
	if _, err := Lights(context.Background(), "bad", hc); !errors.Is(err, ErrKey) {
		t.Errorf("bad key: %v", err)
	}

	d := Device("Govee", "KEY", ls, Options{OnOff: true, Brightness: []int{50}})
	if err := d.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(d.Commands) != 3 || d.Commands[0].Name != "Lamp on" || d.Commands[2].Name != "Lamp 50%" {
		t.Fatalf("%+v", d.Commands)
	}
	sig, err := d.Commands[2].Signal(d)
	if err != nil {
		t.Fatal(err)
	}
	r, err := record.ParseWebRequest(sig)
	if err != nil || r.Method != "PUT" || r.URL != "https://developer-api.govee.com/v1/devices/control" || r.Headers[0] != "Content-Type: application/json" || r.Headers[1] != "Govee-API-Key: KEY" {
		t.Fatalf("%+v, %v", r, err)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(r.Body), &body); err != nil || body["device"] != "AA:BB" || body["cmd"].(map[string]any)["value"] != 50.0 {
		t.Errorf("body %s", r.Body)
	}
}
