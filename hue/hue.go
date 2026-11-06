// Package hue talks to Philips Hue bridges, as the Sofabaton app does to
// add one to a hub: it finds them on the network, gets a user (a key)
// once the bridge's link button is pressed, lists the rooms and lights,
// and makes the commands the hub then sends the bridge (profile.Hue).
//
// It speaks the bridge's version 1 API over plain HTTP, as the hub does.
package hue

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"slices"
	"strconv"
	"strings"

	"github.com/brunoga/sofabaton/internal/mdns"
)

// Bridge is a Hue bridge found on the network.
type Bridge struct {
	Addr netip.Addr
	// ID is the bridge's id, from its announcement; empty if unknown.
	ID string
}

// Discover finds the bridges on the local network until ctx is done, by
// their mDNS announcements (_hue._tcp), as the app does.
func Discover(ctx context.Context) ([]Bridge, error) {
	svcs, err := mdns.Browse(ctx, "_hue._tcp")
	var out []Bridge
	for _, s := range svcs {
		if !s.Addr.Is4() || slices.ContainsFunc(out, func(b Bridge) bool { return b.Addr == s.Addr }) {
			continue
		}
		b := Bridge{Addr: s.Addr}
		for _, kv := range s.TXT {
			if id, ok := strings.CutPrefix(kv, "bridgeid="); ok {
				b.ID = id
			}
		}
		out = append(out, b)
	}
	return out, err
}

// ErrLinkButton is Link's error until the bridge's link button has been
// pressed: the bridge then gives a user for 30 seconds.
var ErrLinkButton = errors.New("hue: press the link button on the bridge first")

// Client is a user of a bridge's API.
type Client struct {
	Addr netip.Addr
	User string
	// HTTP makes the requests; nil for http.DefaultClient.
	HTTP *http.Client
}

// Link asks the bridge at addr for a user for app ("armrest#phone"), which
// it gives within 30 seconds of its link button being pressed; before,
// it fails with ErrLinkButton.
func Link(ctx context.Context, addr netip.Addr, app string, hc *http.Client) (*Client, error) {
	c := &Client{Addr: addr, HTTP: hc}
	body, _ := json.Marshal(map[string]string{"devicetype": app})
	var out []struct {
		Success *struct {
			Username string `json:"username"`
		} `json:"success"`
		Error *apiError `json:"error"`
	}
	if err := c.do(ctx, http.MethodPost, "/api", body, &out); err != nil {
		return nil, err
	}
	for _, r := range out {
		switch {
		case r.Success != nil && r.Success.Username != "":
			c.User = r.Success.Username
			return c, nil
		case r.Error != nil && r.Error.Type == 101:
			return nil, ErrLinkButton
		case r.Error != nil:
			return nil, r.Error
		}
	}
	return nil, errors.New("hue: the bridge gave no user")
}

type apiError struct {
	Type        int    `json:"type"`
	Description string `json:"description"`
}

func (e *apiError) Error() string { return "hue: " + e.Description }

// Target is a room, zone or light of the bridge, which commands switch.
type Target struct {
	// Group is set for a room or zone, unset for a light.
	Group bool
	ID    string
	Name  string
}

// path is the API path, after api/USER/, that changes the target.
func (t Target) path() string {
	if t.Group {
		return "groups/" + t.ID + "/action"
	}
	return "lights/" + t.ID + "/state"
}

// Rooms are the bridge's rooms and zones, in id order.
func (c *Client) Rooms(ctx context.Context) ([]Target, error) {
	var groups map[string]struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}
	if err := c.get(ctx, "groups", &groups); err != nil {
		return nil, err
	}
	var out []Target
	for id, g := range groups {
		if g.Type == "Room" || g.Type == "Zone" {
			out = append(out, Target{Group: true, ID: id, Name: g.Name})
		}
	}
	sortByID(out)
	return out, nil
}

// Lights are the bridge's lights, in id order.
func (c *Client) Lights(ctx context.Context) ([]Target, error) {
	var lights map[string]struct {
		Name string `json:"name"`
	}
	if err := c.get(ctx, "lights", &lights); err != nil {
		return nil, err
	}
	var out []Target
	for id, l := range lights {
		out = append(out, Target{ID: id, Name: l.Name})
	}
	sortByID(out)
	return out, nil
}

func sortByID(ts []Target) {
	slices.SortFunc(ts, func(a, b Target) int {
		x, errA := strconv.Atoi(a.ID)
		y, errB := strconv.Atoi(b.ID)
		if errA == nil && errB == nil {
			return x - y
		}
		return strings.Compare(a.ID, b.ID)
	})
}

// get reads api/USER/what, which the bridge answers with a list of errors
// when it does not know the user.
func (c *Client) get(ctx context.Context, what string, v any) error {
	var raw json.RawMessage
	if err := c.do(ctx, http.MethodGet, "/api/"+c.User+"/"+what, nil, &raw); err != nil {
		return err
	}
	var errs []struct {
		Error *apiError `json:"error"`
	}
	if json.Unmarshal(raw, &errs) == nil {
		for _, e := range errs {
			if e.Error != nil {
				return e.Error
			}
		}
	}
	return json.Unmarshal(raw, v)
}

func (c *Client) do(ctx context.Context, method, path string, body []byte, v any) error {
	req, err := http.NewRequestWithContext(ctx, method, "http://"+c.Addr.String()+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	hc := c.HTTP
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return fmt.Errorf("hue: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("hue: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("hue: %s %s: %s", method, path, resp.Status)
	}
	if err := json.Unmarshal(b, v); err != nil {
		return fmt.Errorf("hue: %s %s: %w", method, path, err)
	}
	return nil
}
