package profile

import (
	"fmt"
	"slices"
	"strings"
)

// The edits below keep a Config consistent: activities name devices and
// commands ("Device: Command"), so renaming or removing one updates every
// reference to it.

// Device is the config's device named name (in any case), or nil.
func (cfg *Config) Device(name string) *Device {
	for _, d := range cfg.Devices {
		if strings.EqualFold(d.Name, name) {
			return d
		}
	}
	return nil
}

// Activity is the config's activity named name (in any case), or nil.
func (cfg *Config) Activity(name string) *Activity {
	for _, a := range cfg.Activities {
		if strings.EqualFold(a.Name, name) {
			return a
		}
	}
	return nil
}

// RenameDevice renames a device, in the activities that use it too.
func (cfg *Config) RenameDevice(old, name string) error {
	d := cfg.Device(old)
	if d == nil {
		return fmt.Errorf("profile: no device %q", old)
	}
	if o := cfg.Device(name); o != nil && o != d {
		return fmt.Errorf("profile: there is a device named %q already", o.Name)
	}
	old, d.Name = d.Name, name
	for _, a := range cfg.Activities {
		a.refs(func(dev, cmd string) (string, string, bool) {
			if strings.EqualFold(dev, old) {
				return name, cmd, true
			}
			return dev, cmd, true
		})
	}
	return nil
}

// RenameCommand renames a device's command, in the device's keys, power,
// inputs and macros and in the activities' keys, steps and favorites.
func (cfg *Config) RenameCommand(device, old, name string) error {
	d := cfg.Device(device)
	if d == nil {
		return fmt.Errorf("profile: no device %q", device)
	}
	i := slices.IndexFunc(d.Commands, func(c Command) bool { return strings.EqualFold(c.Name, old) })
	if i < 0 {
		return fmt.Errorf("profile: device %q has no command %q", d.Name, old)
	}
	if j := slices.IndexFunc(d.Commands, func(c Command) bool { return strings.EqualFold(c.Name, name) }); j >= 0 && j != i {
		return fmt.Errorf("profile: device %q has a command named %q already", d.Name, d.Commands[j].Name)
	}
	old, d.Commands[i].Name = d.Commands[i].Name, name
	d.commandRefs(func(cmd string) string {
		if strings.EqualFold(cmd, old) {
			return name
		}
		return cmd
	})
	for _, a := range cfg.Activities {
		a.refs(func(dev, cmd string) (string, string, bool) {
			if strings.EqualFold(dev, d.Name) && strings.EqualFold(cmd, old) {
				return dev, name, true
			}
			return dev, cmd, true
		})
	}
	return nil
}

// RemoveDevice removes a device, and from the activities that use it: as
// a member, and the keys, steps and favorites that run its commands.
func (cfg *Config) RemoveDevice(name string) error {
	d := cfg.Device(name)
	if d == nil {
		return fmt.Errorf("profile: no device %q", name)
	}
	cfg.Devices = slices.DeleteFunc(cfg.Devices, func(o *Device) bool { return o == d })
	for _, a := range cfg.Activities {
		a.Devices = slices.DeleteFunc(a.Devices, func(m Member) bool { return strings.EqualFold(m.Device, d.Name) })
		a.refs(func(dev, cmd string) (string, string, bool) {
			return dev, cmd, !strings.EqualFold(dev, d.Name)
		})
	}
	return nil
}

// RemoveCommand removes a device's command, and what uses it: the
// device's keys (a long press that uses it goes, a short press takes the
// whole key), power (both of it), inputs and macro steps, and the
// activities' keys, steps and favorites.
func (cfg *Config) RemoveCommand(device, name string) error {
	d := cfg.Device(device)
	if d == nil {
		return fmt.Errorf("profile: no device %q", device)
	}
	i := slices.IndexFunc(d.Commands, func(c Command) bool { return strings.EqualFold(c.Name, name) })
	if i < 0 {
		return fmt.Errorf("profile: device %q has no command %q", d.Name, name)
	}
	name = d.Commands[i].Name
	d.Commands = slices.Delete(d.Commands, i, i+1)
	is := func(cmd string) bool { return strings.EqualFold(cmd, name) }
	for k, key := range d.Buttons {
		switch {
		case is(key.Short):
			delete(d.Buttons, k)
		case is(key.Long):
			d.Buttons[k] = Key{Short: key.Short}
		}
	}
	if d.Power != nil && (is(d.Power.On) || is(d.Power.Off)) {
		d.Power = nil
	}
	if d.Inputs != nil {
		d.Inputs = slices.DeleteFunc(d.Inputs, is)
	}
	for i := range d.Macros {
		d.Macros[i].Steps = slices.DeleteFunc(d.Macros[i].Steps, func(s DeviceStep) bool { return is(s.Command) })
	}
	for _, a := range cfg.Activities {
		a.refs(func(dev, cmd string) (string, string, bool) {
			return dev, cmd, !strings.EqualFold(dev, d.Name) || !is(cmd)
		})
		for j, m := range a.Devices {
			if strings.EqualFold(m.Device, d.Name) && is(m.Input) {
				a.Devices[j].Input = ""
			}
		}
	}
	return nil
}

// RemoveActivity removes an activity.
func (cfg *Config) RemoveActivity(name string) error {
	a := cfg.Activity(name)
	if a == nil {
		return fmt.Errorf("profile: no activity %q", name)
	}
	cfg.Activities = slices.DeleteFunc(cfg.Activities, func(o *Activity) bool { return o == a })
	return nil
}

// refs calls fn on every device and command the activity refers to: its
// members (with no command), keys, steps and favorites. fn returns the new
// device and command, and false to drop the reference: a key, step or
// favorite. Keys and steps running the activity's own macros, and waits,
// are left alone.
func (a *Activity) refs(fn func(dev, cmd string) (string, string, bool)) {
	for i, m := range a.Devices {
		if dev, _, keep := fn(m.Device, ""); keep {
			a.Devices[i].Device = dev
		}
	}
	// "Device: Command", or an activity macro's name.
	ref := func(s string) (string, bool) {
		dev, cmd, ok := strings.Cut(s, ": ")
		if !ok || s == "" {
			return s, true
		}
		dev, cmd, keep := fn(dev, cmd)
		return dev + ": " + cmd, keep
	}
	for k, key := range a.Buttons {
		short, keepShort := ref(key.Short)
		long, keepLong := ref(key.Long)
		if !keepLong {
			long = ""
		}
		if !keepShort {
			short, long = long, ""
		}
		if short == "" {
			delete(a.Buttons, k)
			continue
		}
		a.Buttons[k] = Key{Short: short, Long: long}
	}
	steps := func(ss []Step) []Step {
		out := ss[:0]
		for _, s := range ss {
			if s.Device != "" {
				dev, cmd, keep := fn(s.Device, s.Command)
				if !keep {
					continue
				}
				s.Device, s.Command = dev, cmd
			}
			out = append(out, s)
		}
		return out
	}
	a.PowerOn, a.PowerOff = steps(a.PowerOn), steps(a.PowerOff)
	for i := range a.Macros {
		a.Macros[i].Steps = steps(a.Macros[i].Steps)
	}
	if a.Favorites != nil {
		favs := FavoriteList{}
		for _, f := range a.Favorites {
			if f, keep := ref(f); keep {
				favs = append(favs, f)
			}
		}
		a.Favorites = favs
	}
}

// commandRefs replaces the command names the device's keys, power, inputs
// and macros use with fn's.
func (d *Device) commandRefs(fn func(cmd string) string) {
	for k, key := range d.Buttons {
		key.Short = fn(key.Short)
		if key.Long != "" {
			key.Long = fn(key.Long)
		}
		d.Buttons[k] = key
	}
	if d.Power != nil {
		d.Power.On, d.Power.Off = fn(d.Power.On), fn(d.Power.Off)
	}
	for i, in := range d.Inputs {
		d.Inputs[i] = fn(in)
	}
	for i := range d.Macros {
		for j, s := range d.Macros[i].Steps {
			if s.Command != "" {
				d.Macros[i].Steps[j].Command = fn(s.Command)
			}
		}
	}
}
