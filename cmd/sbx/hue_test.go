package main

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// sbx hue makes a bridge's profile, waiting for its link button, and
// apply creates the device of it.
func TestHue(t *testing.T) {
	simulated(t)
	asked := 0
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api", func(w http.ResponseWriter, r *http.Request) {
		if asked++; asked < 2 {
			_, _ = io.WriteString(w, `[{"error":{"type":101,"description":"link button not pressed"}}]`)
			return
		}
		_, _ = io.WriteString(w, `[{"success":{"username":"KEY"}}]`)
	})
	mux.HandleFunc("GET /api/KEY/groups", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"81":{"name":"Cinema","type":"Room"},"3":{"name":"Backyard","type":"Room"}}`)
	})
	mux.HandleFunc("GET /api/KEY/lights", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"27":{"name":"Pool Table"}}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	hueHTTP = &http.Client{Transport: &http.Transport{DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, srv.Listener.Addr().String())
	}}}
	linkPoll = time.Millisecond
	t.Cleanup(func() { hueHTTP, linkPoll = nil, 2*time.Second })

	file := t.TempDir() + "/hue.yaml"
	var out, errs bytes.Buffer
	if err := run(context.Background(), strings.NewReader(""), []string{"hue", "-bridge", "192.168.1.183", "-only", "cinema, pool table", "-brightness", "50%", "-o", file}, &out, &errs); err != nil {
		t.Fatalf("%v\n%s", err, errs.String())
	}
	if !strings.Contains(errs.String(), "press the link button") || asked != 2 {
		t.Errorf("asked %d times:\n%s", asked, errs.String())
	}
	y, _ := os.ReadFile(file)
	for _, want := range []string{"device: Hue Bridge (192.168.1.183)", "user: KEY", "name: Cinema on", "path: groups/81/action", "name: Pool Table 50% Brightness", "path: lights/27/state"} {
		if !strings.Contains(string(y), want) {
			t.Errorf("no %q in\n%s", want, y)
		}
	}
	if strings.Contains(string(y), "Backyard") {
		t.Errorf("not picked, made:\n%s", y)
	}

	out.Reset()
	if err := run(context.Background(), strings.NewReader("y\n"), []string{"apply", "-hub", "192.0.2.1", "-f", file, "-create"}, &out, &errs); err != nil || !strings.Contains(out.String(), "committed device") {
		t.Fatalf("create: %v\n%s", err, out.String())
	}
	if err := run(context.Background(), strings.NewReader(""), []string{"hue", "-bridge", "192.168.1.183", "-user", "KEY", "-only", "Nowhere"}, &out, &errs); err == nil || !strings.Contains(err.Error(), "nowhere") {
		t.Errorf("an unknown room: %v", err)
	}
}
