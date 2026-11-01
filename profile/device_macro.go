package profile

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/brunoga/sofabaton/record"
	"github.com/brunoga/sofabaton/wire"
)

// A device's macros run its own commands. The remote shows a device's
// own macros (other than power on and off) as one blank entry, so they
// live as copies in every activity with the device instead, the device
// kept in each copy (record.Macro's Origin): applying a device's profile
// writes the copies, and an activity's keys run them as "Device: Macro".
//
//	macros:
//	  - name: 3D On
//	    steps:
//	      - Menu
//	      - wait: 2
//	      - {send: OK, hold: 3}

// MacroList is a device's macros. nil leaves the hub's copies as they
// are; empty means the device has none (and, with prune, removes them).
type MacroList []DeviceMacro

// IsZero keeps an empty list in the YAML: only nil is left out.
func (l MacroList) IsZero() bool { return l == nil }

// UnmarshalYAML reads a list of macros; [] is an empty list, not nil.
func (l *MacroList) UnmarshalYAML(n *yaml.Node) error {
	var macros []DeviceMacro
	if err := n.Decode(&macros); err != nil {
		return err
	}
	if macros == nil {
		macros = []DeviceMacro{}
	}
	*l = macros
	return nil
}

// DeviceMacro is one of a device's macros.
type DeviceMacro struct {
	Name  string       `yaml:"name"`
	Steps []DeviceStep `yaml:"steps"`
}

// DeviceStep is a step of a device's macro: one of its commands
// ("OK", or {send: OK, hold: 3} with the hub's duration byte, meaning
// unknown) or a pause ("wait: 2").
type DeviceStep struct {
	Command string
	Seconds uint8
	Hold    uint8
}

func (s DeviceStep) MarshalYAML() (any, error) {
	scalar := func(v string) *yaml.Node { return &yaml.Node{Kind: yaml.ScalarNode, Value: v} }
	switch {
	case s.Command == "":
		return &yaml.Node{Kind: yaml.MappingNode, Style: yaml.FlowStyle, Content: []*yaml.Node{scalar("wait"), scalar(strconv.Itoa(int(s.Seconds)))}}, nil
	case s.Hold != 0:
		return &yaml.Node{Kind: yaml.MappingNode, Style: yaml.FlowStyle, Content: []*yaml.Node{scalar("send"), scalar(s.Command), scalar("hold"), scalar(strconv.Itoa(int(s.Hold)))}}, nil
	}
	return scalar(s.Command), nil
}

func (s *DeviceStep) UnmarshalYAML(n *yaml.Node) error {
	*s = DeviceStep{}
	if n.Kind == yaml.ScalarNode {
		s.Command = n.Value
		return nil
	}
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("line %d: a step is a command, {wait: N} or {send: Command, hold: N}", n.Line)
	}
	var m map[string]string
	if err := n.Decode(&m); err != nil {
		return fmt.Errorf("line %d: %w", n.Line, err)
	}
	number := func(k string, lo, hi uint64) (uint8, error) {
		v, err := strconv.ParseUint(m[k], 10, 8)
		if err != nil || v < lo || v > hi {
			return 0, fmt.Errorf("line %d: %s %q is not %d-%d", n.Line, k, m[k], lo, hi)
		}
		return uint8(v), nil
	}
	var err error
	switch {
	case len(m) == 1 && m["wait"] != "":
		s.Seconds, err = number("wait", 1, 254)
	case m["send"] != "" && (len(m) == 1 || len(m) == 2 && m["hold"] != ""):
		s.Command = m["send"]
		if m["hold"] != "" {
			s.Hold, err = number("hold", 0, 255)
		}
	default:
		err = fmt.Errorf("line %d: a step is a command, {wait: N} or {send: Command, hold: N}", n.Line)
	}
	return err
}

// validateMacros checks that the macros have names, unique and unlike
// the commands' (an activity's keys name both as "Device: Name"), short
// enough to carry the device in an activity's copy, and steps that send
// commands of the profile.
func (d *Device) validateMacros() []error {
	var errs []error
	seen := map[string]bool{}
	for _, m := range d.Macros {
		key := strings.ToLower(m.Name)
		switch {
		case m.Name == "":
			errs = append(errs, errors.New("profile: a macro has no name"))
			continue
		case seen[key]:
			errs = append(errs, fmt.Errorf("profile: macro %q appears twice", m.Name))
		case d.find(m.Name) >= 0:
			errs = append(errs, fmt.Errorf("profile: macro %q has the name of a command", m.Name))
		}
		seen[key] = true
		probe, _ := record.NewMacro(wire.X2, record.MinActivityID, 1, m.Name)
		probe.Origin = record.MinDeviceID
		if _, err := probe.Body(); err != nil {
			errs = append(errs, fmt.Errorf("profile: macro %q: at most %d characters fit", m.Name, maxMacroName))
		}
		if len(m.Steps) == 0 {
			errs = append(errs, fmt.Errorf("profile: macro %q has no steps", m.Name))
		}
		for i, s := range m.Steps {
			if s.Command != "" && d.find(s.Command) < 0 {
				errs = append(errs, fmt.Errorf("profile: macro %q, step %d: %q is not a command of the profile", m.Name, i+1, s.Command))
			}
		}
	}
	return errs
}

// maxMacroName is the longest name of a device's macro: an X2 label slot
// holds 30 characters, less the terminator and the device.
const maxMacroName = 27

// deviceMacros are the device's macros, from their copies in its
// activities: the first copy of each name. nil when there are none.
func (st *State) deviceMacros(byID map[byte]*record.Command) MacroList {
	var out MacroList
	seen := map[string]bool{}
	for _, a := range st.Activities {
		for _, m := range a.Macros {
			key := strings.ToLower(m.Label)
			if m.Origin != st.Device || seen[key] {
				continue
			}
			seen[key] = true
			dm := DeviceMacro{Name: m.Label}
			for _, s := range m.Steps {
				switch {
				case s.IsDelay():
					dm.Steps = append(dm.Steps, DeviceStep{Seconds: s.Delay})
				case s.Device == st.Device && byID[s.Command] != nil:
					dm.Steps = append(dm.Steps, DeviceStep{Command: byID[s.Command].Label, Hold: s.Duration})
				default:
					dm.Steps = append(dm.Steps, DeviceStep{Command: fmt.Sprintf("#%d", s.Command), Hold: s.Duration})
				}
			}
			out = append(out, dm)
		}
	}
	return out
}

// planCopies plans the copies of the device's macros in its activities:
// a new copy, or a copy rewritten, for each macro whose copy is missing
// or differs; with prune, the removal of copies of macros the profile
// lacks; the new copies' places on the remote's list and a sync. press
// names the device's commands by their ids to be. A nil Macros leaves
// the copies as they are.
func (d *Device) planCopies(st *State, press func(string) record.Press, prune bool) ([]Change, error) {
	if d.Macros == nil {
		return nil, nil
	}
	var out []Change
	for _, a := range st.Activities {
		act := a.Entity.ID
		var acts []ActivityChange
		copies, own := map[string]*record.Macro{}, map[string]bool{}
		for _, m := range a.Macros {
			switch {
			case m.Origin == st.Device:
				copies[strings.ToLower(m.Label)] = m
			case m.Origin == 0 && m.ID != record.MacroPowerOn && m.ID != record.MacroPowerOff:
				own[strings.ToLower(m.Label)] = true
			}
		}
		wanted := map[string]bool{}
		var added []string
		for _, dm := range d.Macros {
			key := strings.ToLower(dm.Name)
			wanted[key] = true
			if own[key] {
				return nil, fmt.Errorf("profile: macro %q: activity %q has a macro of its own of that name", dm.Name, a.Entity.Name)
			}
			var steps []record.Step
			for _, s := range dm.Steps {
				if s.Command == "" {
					steps = append(steps, waitStep(s.Seconds))
					continue
				}
				p := press(s.Command)
				steps = append(steps, record.Step{Device: st.Device, Command: p.Command, Code: p.Code, Duration: s.Hold, Delay: 0xFF})
			}
			if m := copies[key]; m != nil {
				if !sameSteps(m.Steps, steps) || m.Label != dm.Name {
					cp := *m
					cp.Steps, cp.Label = steps, dm.Name
					acts = append(acts, ActivityChange{Kind: ActMacro, Activity: act, Macro: &cp, Name: d.Name})
				}
				continue
			}
			m, err := record.NewMacro(st.Variant, act, 0, dm.Name)
			if err != nil {
				return nil, err
			}
			m.Origin, m.Steps = st.Device, steps
			acts = append(acts, ActivityChange{Kind: ActNewMacro, Activity: act, Macro: m, Name: d.Name})
			added = append(added, macroKey(m))
		}
		var removed []byte
		for key, m := range copies {
			if prune && !wanted[key] {
				acts = append(acts, ActivityChange{Kind: ActRemoveMacro, Activity: act, Macro: m, Name: d.Name})
				removed = append(removed, m.ID)
			}
		}
		if len(added) > 0 {
			acts = append(acts, ActivityChange{Kind: ActOrder, Activity: act, Order: appendOrder(a.Macros, a.Order, removed, added)})
		}
		if len(acts) > 0 {
			acts = append(acts, ActivityChange{Kind: ActSync, Activity: act})
		}
		for _, ac := range acts {
			out = append(out, Change{Kind: InActivity, Activity: a.Entity.Name, Act: ac})
		}
	}
	return out, nil
}

// appendOrder is an activity's list on the remote with new macros (by
// macroKey) at its end: the list as placed, or, when the activity has
// none, its macros by id; less the removed.
func appendOrder(macros []*record.Macro, order []record.Place, removed []byte, added []string) []OrderEntry {
	var out []OrderEntry
	if len(order) > 0 {
		placed := slices.Clone(order)
		slices.SortStableFunc(placed, func(x, y record.Place) int { return int(x.Position) - int(y.Position) })
		for _, p := range placed {
			if p.Placed() && !slices.Contains(removed, p.Command) {
				out = append(out, OrderEntry{ID: p.Command})
			}
		}
	} else {
		var ids []byte
		for _, m := range macros {
			if m.ID != record.MacroPowerOn && m.ID != record.MacroPowerOff && !slices.Contains(removed, m.ID) {
				ids = append(ids, m.ID)
			}
		}
		slices.Sort(ids)
		for _, id := range ids {
			out = append(out, OrderEntry{ID: id})
		}
	}
	for _, k := range added {
		out = append(out, OrderEntry{Copy: k})
	}
	return out
}
