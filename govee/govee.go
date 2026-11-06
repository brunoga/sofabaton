// Package govee lists a Govee account's lights and makes a device of
// commands for them, as the Sofabaton app does: requests to Govee's cloud
// API, with the account's API key, which the X2 sends itself over HTTPS
// (profile.HTTP, in the X2's newer form).
package govee

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/brunoga/sofabaton/profile"
)

// API is Govee's developer API, the one the app uses; tests replace it.
var API = "https://developer-api.govee.com/v1"

// Light is a Govee device of the account.
type Light struct {
	// Device and Model are what the API names it by.
	Device string `json:"device"`
	Model  string `json:"model"`
	Name   string `json:"deviceName"`
	// Commands are the commands it takes ("turn", "brightness", ...).
	Commands []string `json:"supportCmds"`
}

// ErrKey is Lights's error for a key Govee does not take.
var ErrKey = errors.New("govee: the API key is not valid (Govee Home: Profile, Settings, Apply for API Key)")

// Lights lists the account's devices that can be controlled; hc nil is
// http.DefaultClient.
func Lights(ctx context.Context, key string, hc *http.Client) ([]Light, error) {
	if hc == nil {
		hc = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, API+"/devices", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Govee-API-Key", key)
	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("govee: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("govee: %w", err)
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, ErrKey
	}
	var v struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Devices []struct {
				Light
				Controllable bool `json:"controllable"`
			} `json:"devices"`
		} `json:"data"`
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return nil, fmt.Errorf("govee: %s: %w", resp.Status, err)
	}
	if resp.StatusCode != http.StatusOK || v.Code != 200 {
		return nil, fmt.Errorf("govee: %s: %s", resp.Status, v.Message)
	}
	var out []Light
	for _, d := range v.Data.Devices {
		if d.Controllable {
			out = append(out, d.Light)
		}
	}
	return out, nil
}

// Brightness levels the app offers, in percent.
var Brightness = []int{10, 50, 80, 100}

// Options say which commands Device makes for each light.
type Options struct {
	OnOff bool
	// Brightness makes a command for each level, in percent (1-100).
	Brightness []int
}

// Device is the profile of a device named name of commands for lights,
// sent with key: on and off and each brightness level for each light, as
// the app makes them, named "Lamp on", "Lamp 50%". Names the hub cannot
// hold have the light's name cut; one taken already gets a number.
func Device(name, key string, lights []Light, o Options) *profile.Device {
	d := &profile.Device{Name: name, Brand: "Govee"}
	seen := map[string]bool{}
	add := func(l Light, what string, cmd any) {
		n := profile.FitName(l.Name, what)
		for i := 2; seen[strings.ToLower(n)]; i++ {
			n = profile.FitName(l.Name, fmt.Sprint(what, " ", i))
		}
		seen[strings.ToLower(n)] = true
		body, _ := json.Marshal(struct {
			Device string `json:"device"`
			Model  string `json:"model"`
			Cmd    any    `json:"cmd"`
		}{l.Device, l.Model, cmd})
		d.Commands = append(d.Commands, profile.Command{Name: n, HTTP: &profile.HTTP{
			Method: http.MethodPut, URL: API + "/devices/control",
			Headers: []string{"Govee-API-Key: " + key}, ContentType: "application/json", Body: string(body),
		}})
	}
	type command struct {
		Name  string `json:"name"`
		Value any    `json:"value"`
	}
	for _, l := range lights {
		if o.OnOff {
			add(l, " on", command{"turn", "on"})
			add(l, " off", command{"turn", "off"})
		}
		for _, pct := range o.Brightness {
			pct = max(1, min(100, pct))
			add(l, fmt.Sprintf(" %d%%", pct), command{"brightness", pct})
		}
	}
	return d
}
