package profile

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/brunoga/sofabaton/hub"
	"github.com/brunoga/sofabaton/record"
)

// Restore makes the hub dst hold what src holds (a hub, or a dump served
// by dump.File.Open): the profile of each of src's devices and
// activities, matched to dst's by name, is applied to the match or, if
// there is none, creates one. With prune, a match loses what src's
// profile lacks (commands, macros, keys, favorites); dst's devices and
// activities that src lacks are kept, and reported. With dryRun nothing
// is written: report gets the plans, and the creates as one line each.
//
// Only IR devices can be created, of commands with a code or descriptor
// (see Create); others are skipped, with the activities that need them.
// A device's macros live in copies in its activities (see MacroList), so
// a new activity first gets every key but those running them, the devices
// then write their copies, and the activity's keys are applied again.
func Restore(ctx context.Context, src, dst *hub.Client, prune, dryRun bool, report func(string)) error {
	r := restorer{ctx: ctx, src: src, dst: dst, prune: prune, dry: dryRun, report: report}
	return r.run()
}

type restorer struct {
	ctx      context.Context
	src, dst *hub.Client
	prune    bool
	dry      bool
	report   func(string)

	devices    []*Device   // src's, in its order
	activities []*Activity // src's, in its order
	skipped    map[string]bool
}

func (r *restorer) say(format string, args ...any) {
	if r.report != nil {
		r.report(fmt.Sprintf(format, args...))
	}
}

func (r *restorer) run() error {
	if err := r.read(); err != nil {
		return err
	}
	r.skipped = map[string]bool{}
	for _, d := range r.devices {
		if err := r.device(d, false); err != nil {
			return err
		}
	}
	// Device macros run from keys only once their copies exist.
	held := map[string]*Activity{}
	for _, a := range r.activities {
		full := a
		if a2, ok := r.withoutCopyKeys(a); ok {
			held[strings.ToLower(a.Name)], a = full, a2
		}
		if err := r.activity(a); err != nil {
			return err
		}
	}
	if r.dry {
		return nil
	}
	for _, d := range r.devices {
		if len(d.Macros) > 0 && !r.skipped[strings.ToLower(d.Name)] {
			if err := r.device(d, true); err != nil {
				return err
			}
		}
	}
	for _, a := range r.activities {
		if full := held[strings.ToLower(a.Name)]; full != nil && !r.skipped[strings.ToLower(a.Name)] {
			if err := r.activity(full); err != nil {
				return err
			}
		}
	}
	return r.extras()
}

// read builds the profiles of src's devices and activities.
func (r *restorer) read() error {
	devs, err := r.src.Devices(r.ctx)
	if err != nil {
		return fmt.Errorf("profile: restore: reading the source: %w", err)
	}
	for _, e := range devs {
		st, err := Read(r.ctx, r.src, e.ID)
		if err != nil {
			return fmt.Errorf("profile: restore: reading device %q: %w", e.Name, err)
		}
		d, err := FromHub(st.Entity, st)
		if err != nil {
			return fmt.Errorf("profile: restore: device %q: %w", e.Name, err)
		}
		r.devices = append(r.devices, d)
	}
	acts, err := r.src.Activities(r.ctx)
	if err != nil && !errors.Is(err, hub.ErrNotFound) {
		return fmt.Errorf("profile: restore: reading the source: %w", err)
	}
	for _, e := range acts {
		st, err := ReadActivity(r.ctx, r.src, e.ID)
		if err != nil {
			return fmt.Errorf("profile: restore: reading activity %q: %w", e.Name, err)
		}
		a, err := ActivityFromHub(st)
		if err != nil {
			return fmt.Errorf("profile: restore: activity %q: %w", e.Name, err)
		}
		r.activities = append(r.activities, a)
	}
	return nil
}

// device applies d to its match on dst, or creates it. again is the
// second pass, for the device's macros.
func (r *restorer) device(d *Device, again bool) error {
	devs, err := r.dst.Devices(r.ctx)
	if err != nil {
		return err
	}
	var match *record.Entity
	for _, e := range devs {
		if strings.EqualFold(e.Name, d.Name) {
			match = e
		}
	}
	if match == nil {
		return r.createDevice(d)
	}
	st, err := Read(r.ctx, r.dst, match.ID)
	if err != nil {
		return fmt.Errorf("profile: restore: device %q: %w", d.Name, err)
	}
	cp := *d
	cp.ID = match.ID
	changes, _, err := Plan(&cp, st, r.prune)
	if err != nil {
		return fmt.Errorf("profile: restore: device %q: %w", d.Name, err)
	}
	if len(changes) == 0 {
		if !again {
			r.say("device %q (%d): matches", d.Name, match.ID)
		}
		return nil
	}
	r.say("device %q (%d): %d changes", d.Name, match.ID, len(changes))
	if r.dry {
		for _, ch := range changes {
			r.say("  %v", ch)
		}
		return nil
	}
	return Apply(r.ctx, r.dst, changes, func(ch Change) { r.say("  %v", ch) })
}

func (r *restorer) createDevice(d *Device) error {
	src := r.sourceEntity(d.Name)
	if src != nil && src.Class != record.ClassIR {
		r.say("device %q: skipped: only IR devices can be created (it is class %v)", d.Name, src.Class)
		r.skipped[strings.ToLower(d.Name)] = true
		return nil
	}
	var data []string
	for _, c := range d.Commands {
		if c.Data != "" {
			data = append(data, c.Name)
		}
	}
	if len(data) > 0 {
		r.say("device %q: skipped: commands without a code or descriptor cannot be created yet (%s)", d.Name, strings.Join(data, ", "))
		r.skipped[strings.ToLower(d.Name)] = true
		return nil
	}
	if r.dry {
		r.say("device %q: create, %d commands", d.Name, len(d.Commands))
		return nil
	}
	cp := *d
	cp.ID = 0
	id, err := Create(r.ctx, r.dst, &cp, func(s string) { r.say("  %s", s) })
	if err != nil {
		return fmt.Errorf("profile: restore: creating device %q: %w", d.Name, err)
	}
	r.say("device %q: created as %d", d.Name, id)
	return nil
}

func (r *restorer) sourceEntity(name string) *record.Entity {
	devs, err := r.src.Devices(r.ctx)
	if err != nil {
		return nil
	}
	for _, e := range devs {
		if strings.EqualFold(e.Name, name) {
			return e
		}
	}
	return nil
}

// withoutCopyKeys is a with the keys that run its devices' macros left
// out, if it has any.
func (r *restorer) withoutCopyKeys(a *Activity) (*Activity, bool) {
	macro := map[string]bool{}
	for _, d := range r.devices {
		for _, m := range d.Macros {
			macro[strings.ToLower(d.Name+": "+m.Name)] = true
		}
	}
	runsCopy := func(k Key) bool { return macro[strings.ToLower(k.Short)] || macro[strings.ToLower(k.Long)] }
	var held bool
	for _, k := range a.Buttons {
		held = held || runsCopy(k)
	}
	if !held {
		return a, false
	}
	cp := *a
	cp.Buttons = Buttons{}
	for name, k := range a.Buttons {
		if !runsCopy(k) {
			cp.Buttons[name] = k
		}
	}
	return &cp, true
}

// activity applies a to its match on dst, or creates it.
func (r *restorer) activity(a *Activity) error {
	for _, m := range a.Devices {
		if r.skipped[strings.ToLower(m.Device)] {
			r.say("activity %q: skipped: its device %q was", a.Name, m.Device)
			r.skipped[strings.ToLower(a.Name)] = true
			return nil
		}
	}
	acts, err := r.dst.Activities(r.ctx)
	if err != nil && !errors.Is(err, hub.ErrNotFound) {
		return err
	}
	var match *record.Entity
	for _, e := range acts {
		if strings.EqualFold(e.Name, a.Name) {
			match = e
		}
	}
	cp := *a
	if match == nil {
		if r.dry {
			r.say("activity %q: create, with %d devices", a.Name, len(a.Devices))
			return nil
		}
		cp.ID = 0
		id, err := CreateActivity(r.ctx, r.dst, &cp, func(s string) { r.say("  %s", s) })
		if err != nil {
			return fmt.Errorf("profile: restore: creating activity %q: %w", a.Name, err)
		}
		r.say("activity %q: created as %d", a.Name, id)
		return nil
	}
	st, err := ReadActivity(r.ctx, r.dst, match.ID)
	if err != nil {
		return fmt.Errorf("profile: restore: activity %q: %w", a.Name, err)
	}
	cp.ID = match.ID
	changes, err := PlanActivity(&cp, st, r.prune)
	if err != nil {
		if r.dry {
			// Its devices may be ones the restore would create.
			r.say("activity %q (%d): cannot be planned before the devices are restored: %v", a.Name, match.ID, err)
			return nil
		}
		return fmt.Errorf("profile: restore: activity %q: %w", a.Name, err)
	}
	if len(changes) == 0 {
		r.say("activity %q (%d): matches", a.Name, match.ID)
		return nil
	}
	r.say("activity %q (%d): %d changes", a.Name, match.ID, len(changes))
	if r.dry {
		for _, ch := range changes {
			r.say("  %v", ch)
		}
		return nil
	}
	return ApplyActivity(r.ctx, r.dst, changes, func(ch ActivityChange) { r.say("  %v", ch) })
}

// extras reports dst's devices and activities that src lacks.
func (r *restorer) extras() error {
	known := map[string]bool{}
	for _, d := range r.devices {
		known[strings.ToLower(d.Name)] = true
	}
	for _, a := range r.activities {
		known[strings.ToLower(a.Name)] = true
	}
	devs, err := r.dst.Devices(r.ctx)
	if err != nil {
		return err
	}
	acts, err := r.dst.Activities(r.ctx)
	if err != nil && !errors.Is(err, hub.ErrNotFound) {
		return err
	}
	for _, e := range append(devs, acts...) {
		if !known[strings.ToLower(e.Name)] {
			what := "device"
			if record.IsActivityID(e.ID) {
				what = "activity"
			}
			r.say("%s %q (%d): not in the source, kept", what, e.Name, e.ID)
		}
	}
	return nil
}
