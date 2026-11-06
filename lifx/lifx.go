// Package lifx lists a LIFX account's lights and makes a device of
// commands for them, as the Sofabaton app does: requests to LIFX's cloud
// API, with the account's token, which the X2 sends itself over HTTPS
// (profile.HTTP, in the X2's newer form). The app's commands are for all
// the lights at once; Device also makes them for each group and light.
package lifx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/brunoga/sofabaton/profile"
)

// API is LIFX's HTTP API; tests replace it.
var API = "https://api.lifx.com/v1"

// Target is what a command controls: all the lights, a group, or a
// light, by its LIFX selector.
type Target struct {
	// Selector is "all", "group_id:ID" or "id:ID".
	Selector string
	Name     string
}

// All is every light of the account, the app's one target.
var All = Target{Selector: "all", Name: "All lights"}

// ErrToken is Targets's error for a token LIFX does not take.
var ErrToken = errors.New("lifx: the token is not valid (cloud.lifx.com: Settings, Personal access tokens)")

// Targets lists the account's groups and lights, groups first, each by
// name; hc nil is http.DefaultClient.
func Targets(ctx context.Context, token string, hc *http.Client) (groups, lights []Target, err error) {
	if hc == nil {
		hc = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, API+"/lights/all", nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := hc.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("lifx: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, nil, fmt.Errorf("lifx: %w", err)
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, nil, ErrToken
	}
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(b, &e)
		return nil, nil, fmt.Errorf("lifx: %s: %s", resp.Status, e.Error)
	}
	var v []struct {
		ID    string `json:"id"`
		Label string `json:"label"`
		Group struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"group"`
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return nil, nil, fmt.Errorf("lifx: %w", err)
	}
	seen := map[string]bool{}
	for _, l := range v {
		if l.Group.ID != "" && !seen[l.Group.ID] {
			seen[l.Group.ID] = true
			groups = append(groups, Target{Selector: "group_id:" + l.Group.ID, Name: l.Group.Name})
		}
		if l.ID != "" {
			lights = append(lights, Target{Selector: "id:" + l.ID, Name: l.Label})
		}
	}
	byName := func(ts []Target) {
		sort.SliceStable(ts, func(i, j int) bool { return strings.ToLower(ts[i].Name) < strings.ToLower(ts[j].Name) })
	}
	byName(groups)
	byName(lights)
	return groups, lights, nil
}

// Brightness levels the app offers, in percent.
var Brightness = []int{10, 50, 80, 100}

// Options say which commands Device makes for each target.
type Options struct {
	OnOff bool
	// Brightness makes a command for each level, in percent (1-100).
	Brightness []int
}

// Device is the profile of a device named name of commands for targets,
// sent with token: on and off and each brightness level for each
// target, named "Kitchen on", "Kitchen 50%". Names the hub cannot hold
// have the target's name cut; one taken already gets a number.
func Device(name, token string, targets []Target, o Options) *profile.Device {
	d := &profile.Device{Name: name, Brand: "LIFX"}
	seen := map[string]bool{}
	add := func(t Target, what, body string) {
		n := profile.FitName(t.Name, what)
		for i := 2; seen[strings.ToLower(n)]; i++ {
			n = profile.FitName(t.Name, fmt.Sprint(what, " ", i))
		}
		seen[strings.ToLower(n)] = true
		d.Commands = append(d.Commands, profile.Command{Name: n, HTTP: &profile.HTTP{
			Method: http.MethodPut, URL: API + "/lights/" + url.PathEscape(t.Selector) + "/state",
			Headers: []string{"Authorization: Bearer " + token}, ContentType: "application/json", Body: body,
		}})
	}
	for _, t := range targets {
		if o.OnOff {
			add(t, " on", `{"power":"on"}`)
			add(t, " off", `{"power":"off"}`)
		}
		for _, pct := range o.Brightness {
			pct = max(1, min(100, pct))
			add(t, fmt.Sprintf(" %d%%", pct), `{"power":"on","brightness":`+strconv.FormatFloat(float64(pct)/100, 'f', -1, 64)+`}`)
		}
	}
	return d
}
