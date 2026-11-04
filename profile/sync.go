package profile

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/brunoga/sofabaton/hub"
	"github.com/brunoga/sofabaton/record"
)

// Config is a hub's whole configuration, as profiles: its devices and
// activities, in the hub's order.
type Config struct {
	Devices    []*Device
	Activities []*Activity
}

// Progress is how far a long operation is: Done of Total steps, and the
// one being done.
type Progress struct {
	Step        string
	Done, Total int
}

// Snapshot reads the whole configuration of the hub c is connected to.
// progress, if not nil, is called before each device and activity is
// read. Reading a real hub takes about two seconds per device or activity;
// ctx can cancel it between them.
func Snapshot(ctx context.Context, c *hub.Client, progress func(Progress)) (*Config, error) {
	devs, err := c.Devices(ctx)
	if err != nil {
		return nil, fmt.Errorf("profile: snapshot: %w", err)
	}
	acts, err := c.Activities(ctx)
	if err != nil && !errors.Is(err, hub.ErrNotFound) {
		return nil, fmt.Errorf("profile: snapshot: %w", err)
	}
	total := len(devs) + len(acts)
	step := func(done int, what string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if progress != nil {
			progress(Progress{Step: what, Done: done, Total: total})
		}
		return nil
	}
	cfg := &Config{}
	for i, e := range devs {
		if err := step(i, "Reading device "+e.Name); err != nil {
			return nil, err
		}
		st, err := Read(ctx, c, e.ID)
		if err != nil {
			return nil, fmt.Errorf("profile: snapshot: device %q: %w", e.Name, err)
		}
		d, err := FromHub(st.Entity, st)
		if err != nil {
			return nil, fmt.Errorf("profile: snapshot: device %q: %w", e.Name, err)
		}
		cfg.Devices = append(cfg.Devices, d)
	}
	for i, e := range acts {
		if err := step(len(devs)+i, "Reading activity "+e.Name); err != nil {
			return nil, err
		}
		st, err := ReadActivity(ctx, c, e.ID)
		if err != nil {
			return nil, fmt.Errorf("profile: snapshot: activity %q: %w", e.Name, err)
		}
		a, err := ActivityFromHub(st)
		if err != nil {
			return nil, fmt.Errorf("profile: snapshot: activity %q: %w", e.Name, err)
		}
		cfg.Activities = append(cfg.Activities, a)
	}
	if progress != nil {
		progress(Progress{Step: "Done", Done: total, Total: total})
	}
	return cfg, nil
}

// ApplyOptions tunes ApplyConfig.
type ApplyOptions struct {
	// ByID matches the config's devices and activities to the hub's by ID
	// first, then by name: for a config read from this hub and edited, so
	// renames are renames. Without it they match by name only, as when
	// restoring onto another hub.
	ByID bool
	// Prune removes from a matched device or activity what its profile
	// lacks (commands, macros, keys, favorites).
	Prune bool
	// Delete deletes the hub's devices and activities the config lacks.
	// Without it they are kept, and reported.
	Delete bool
	// DryRun writes nothing: Report gets the plans, and creates and
	// deletes as one line each. An activity whose devices the sync would
	// create cannot be planned until they exist, and says so.
	DryRun bool
	// Report, if not nil, gets a line per change, as it is made.
	Report func(string)
	// Base, if not nil, is the configuration the hub held when cfg was
	// read from it. The devices and activities cfg left as they were are
	// taken to be on the hub as they are, and not read: reading a device
	// takes the hub seconds. An activity counts as left only if its
	// devices are, and a device only if its activities are (its macros
	// live in copies in them).
	Base *Config
}

// ApplyConfig makes the hub c is connected to hold cfg: each device and
// activity of cfg is applied to its match on the hub (see
// ApplyOptions.ByID) or, if there is none, created. Devices are written
// before the activities that use them; with Delete, the activities the
// config lacks go first and the devices it lacks last. ctx can cancel it
// between changes; what was done stays done.
//
// Only IR devices can be created, of commands with a code or descriptor
// (see Create); others are skipped, with the activities that need them.
// A device's macros live in copies in its activities (see MacroList), so
// a new activity first gets every key but those running them, the devices
// then write their copies, and the activity's keys are applied again.
func ApplyConfig(ctx context.Context, c *hub.Client, cfg *Config, opts ApplyOptions) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	s := syncer{ctx: ctx, c: c, cfg: cfg, opts: opts, skipped: map[string]bool{}, settled: map[string]bool{}}
	s.settle()
	return s.run()
}

// Validate checks that names are unique and that every activity's devices
// are in the config.
func (cfg *Config) Validate() error {
	devs := map[string]bool{}
	for _, d := range cfg.Devices {
		k := strings.ToLower(d.Name)
		if devs[k] {
			return fmt.Errorf("profile: two devices are named %q", d.Name)
		}
		devs[k] = true
	}
	acts := map[string]bool{}
	for _, a := range cfg.Activities {
		k := strings.ToLower(a.Name)
		if acts[k] {
			return fmt.Errorf("profile: two activities are named %q", a.Name)
		}
		acts[k] = true
		for _, m := range a.Devices {
			if !devs[strings.ToLower(m.Device)] {
				return fmt.Errorf("profile: activity %q uses device %q, which is not in the configuration", a.Name, m.Device)
			}
		}
	}
	return nil
}

type syncer struct {
	ctx     context.Context
	c       *hub.Client
	cfg     *Config
	opts    ApplyOptions
	skipped map[string]bool // devices and activities skipped, by lower-case name
	settled map[string]bool // devices and activities as in Base, by lower-case name
}

// settle finds the devices and activities cfg left as they are in Base.
func (s *syncer) settle() {
	base := s.opts.Base
	if base == nil {
		return
	}
	devs := map[string]bool{}
	for _, d := range s.cfg.Devices {
		for _, b := range base.Devices {
			if s.same(d.ID, b.ID, d.Name, b.Name) {
				devs[strings.ToLower(d.Name)] = sameProfile(d, b)
				break
			}
		}
	}
	acts := map[string]bool{}
	for _, a := range s.cfg.Activities {
		for _, b := range base.Activities {
			if s.same(a.ID, b.ID, a.Name, b.Name) {
				ok := sameProfile(a, b)
				for _, m := range a.Devices {
					ok = ok && devs[strings.ToLower(m.Device)]
				}
				acts[strings.ToLower(a.Name)] = ok
				break
			}
		}
	}
	for name, ok := range devs {
		for _, a := range s.cfg.Activities {
			for _, m := range a.Devices {
				if strings.EqualFold(m.Device, name) {
					ok = ok && acts[strings.ToLower(a.Name)]
				}
			}
		}
		s.settled[name] = ok
	}
	for name, ok := range acts {
		s.settled[name] = ok
	}
}

// sameProfile reports whether two profiles say the same, as written: a
// copy made through YAML (as an editor's) is the same profile.
func sameProfile(a, b any) bool {
	x, err1 := yaml.Marshal(a)
	y, err2 := yaml.Marshal(b)
	return err1 == nil && err2 == nil && bytes.Equal(x, y)
}

// same reports whether a profile and one of Base are the same device or
// activity, matched as on the hub (see match).
func (s *syncer) same(id, baseID byte, name, baseName string) bool {
	if s.opts.ByID && id != 0 {
		return id == baseID
	}
	return strings.EqualFold(name, baseName)
}

func (s *syncer) say(format string, args ...any) {
	if s.opts.Report != nil {
		s.opts.Report(fmt.Sprintf(format, args...))
	}
}

func (s *syncer) run() error {
	// Activities go first, and devices last: the activities kept may stop
	// using a device that goes.
	var goneDevs []*record.Entity
	if s.opts.Delete {
		devs, acts, err := s.unmatched()
		if err != nil {
			return err
		}
		for _, e := range acts {
			if err := s.delete(e, "activity", s.c.DeleteActivity); err != nil {
				return err
			}
		}
		goneDevs = devs
	}
	for _, d := range s.cfg.Devices {
		if err := s.device(d, false); err != nil {
			return err
		}
	}
	// Device macros run from keys only once their copies exist.
	held := map[string]*Activity{}
	for _, a := range s.cfg.Activities {
		pending, err := s.activity(a, true)
		if err != nil {
			return err
		}
		if pending {
			held[strings.ToLower(a.Name)] = a
		}
	}
	if !s.opts.DryRun {
		if err := s.macroPass(held); err != nil {
			return err
		}
	}
	for _, e := range goneDevs {
		if err := s.delete(e, "device", s.c.DeleteDevice); err != nil {
			return err
		}
	}
	return s.extras()
}

// macroPass writes the devices' macro copies into the activities, now
// that they have the devices, then the keys held back that run them.
func (s *syncer) macroPass(held map[string]*Activity) error {
	for _, d := range s.cfg.Devices {
		if len(d.Macros) > 0 && !s.skipped[strings.ToLower(d.Name)] {
			if err := s.device(d, true); err != nil {
				return err
			}
		}
	}
	for _, a := range s.cfg.Activities {
		if full := held[strings.ToLower(a.Name)]; full != nil && !s.skipped[strings.ToLower(a.Name)] {
			if _, err := s.activity(full, false); err != nil {
				return err
			}
		}
	}
	return nil
}

// match is the hub's entity for a profile's id and name, or nil.
func (s *syncer) match(ents []*record.Entity, id byte, name string) *record.Entity {
	if s.opts.ByID && id != 0 {
		for _, e := range ents {
			if e.ID == id {
				return e
			}
		}
		return nil
	}
	for _, e := range ents {
		if strings.EqualFold(e.Name, name) {
			return e
		}
	}
	return nil
}

func (s *syncer) devices() ([]*record.Entity, error) {
	if err := s.ctx.Err(); err != nil {
		return nil, err
	}
	return s.c.Devices(s.ctx)
}

func (s *syncer) activities() ([]*record.Entity, error) {
	if err := s.ctx.Err(); err != nil {
		return nil, err
	}
	acts, err := s.c.Activities(s.ctx)
	if errors.Is(err, hub.ErrNotFound) {
		err = nil
	}
	return acts, err
}

// unmatched is the hub's entities that none of the config's profiles
// match.
func (s *syncer) unmatched() (devs, acts []*record.Entity, err error) {
	hubDevs, err := s.devices()
	if err != nil {
		return nil, nil, err
	}
	hubActs, err := s.activities()
	if err != nil {
		return nil, nil, err
	}
	matched := map[byte]bool{}
	for _, d := range s.cfg.Devices {
		if e := s.match(hubDevs, d.ID, d.Name); e != nil {
			matched[e.ID] = true
		}
	}
	for _, a := range s.cfg.Activities {
		if e := s.match(hubActs, a.ID, a.Name); e != nil {
			matched[e.ID] = true
		}
	}
	for _, e := range hubDevs {
		if !matched[e.ID] {
			devs = append(devs, e)
		}
	}
	for _, e := range hubActs {
		if !matched[e.ID] {
			acts = append(acts, e)
		}
	}
	return devs, acts, nil
}

func (s *syncer) delete(e *record.Entity, what string, del func(context.Context, byte) error) error {
	if s.opts.DryRun {
		s.say("%s %q (%d): delete", what, e.Name, e.ID)
		return nil
	}
	if err := s.ctx.Err(); err != nil {
		return err
	}
	if err := del(s.ctx, e.ID); err != nil {
		return fmt.Errorf("profile: deleting %s %q: %w", what, e.Name, err)
	}
	s.say("%s %q (%d): deleted", what, e.Name, e.ID)
	return nil
}

// device applies d to its match on the hub, or creates it. again is the
// second pass, for the device's macros.
func (s *syncer) device(d *Device, again bool) error {
	if s.settled[strings.ToLower(d.Name)] {
		if !again {
			s.say("device %q (%d): matches", d.Name, d.ID)
		}
		return nil
	}
	devs, err := s.devices()
	if err != nil {
		return err
	}
	match := s.match(devs, d.ID, d.Name)
	if match == nil {
		if again {
			return nil
		}
		return s.createDevice(d)
	}
	st, err := Read(s.ctx, s.c, match.ID)
	if err != nil {
		return fmt.Errorf("profile: device %q: %w", d.Name, err)
	}
	cp := *d
	cp.ID = match.ID
	changes, _, err := Plan(&cp, st, s.opts.Prune)
	if err != nil {
		return fmt.Errorf("profile: device %q: %w", d.Name, err)
	}
	if len(changes) == 0 {
		if !again {
			s.say("device %q (%d): matches", d.Name, match.ID)
		}
		return nil
	}
	s.say("device %q (%d): %d changes", d.Name, match.ID, len(changes))
	if s.opts.DryRun {
		for _, ch := range changes {
			s.say("  %v", ch)
		}
		return nil
	}
	return Apply(s.ctx, s.c, changes, func(ch Change) { s.say("  %v", ch) })
}

func (s *syncer) createDevice(d *Device) error {
	if d.class != 0 && d.class != record.ClassIR {
		s.say("device %q: skipped: only IR devices can be created (it is class %v)", d.Name, d.class)
		s.skipped[strings.ToLower(d.Name)] = true
		return nil
	}
	var data []string
	for _, c := range d.Commands {
		if c.Data != "" {
			data = append(data, c.Name)
		}
	}
	if len(data) > 0 {
		s.say("device %q: skipped: commands without a code or descriptor cannot be created yet (%s)", d.Name, strings.Join(data, ", "))
		s.skipped[strings.ToLower(d.Name)] = true
		return nil
	}
	if s.opts.DryRun {
		s.say("device %q: create, %d commands", d.Name, len(d.Commands))
		return nil
	}
	if err := s.ctx.Err(); err != nil {
		return err
	}
	cp := *d
	cp.ID = 0
	id, err := Create(s.ctx, s.c, &cp, func(line string) { s.say("  %s", line) })
	if err != nil {
		return fmt.Errorf("profile: creating device %q: %w", d.Name, err)
	}
	s.say("device %q: created as %d", d.Name, id)
	return nil
}

// withoutCopyKeys is a with the keys that run its devices' macros left
// out, if it has any.
func (s *syncer) withoutCopyKeys(a *Activity) (*Activity, bool) {
	macro := map[string]bool{}
	for _, d := range s.cfg.Devices {
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

// activity applies a to its match on the hub, or creates it. With hold,
// the keys running its devices' macros are held back if their copies are
// not in the activity yet (see ApplyConfig); pending reports that they were.
func (s *syncer) activity(a *Activity, hold bool) (pending bool, err error) {
	for _, m := range a.Devices {
		if s.skipped[strings.ToLower(m.Device)] {
			s.say("activity %q: skipped: its device %q was", a.Name, m.Device)
			s.skipped[strings.ToLower(a.Name)] = true
			return false, nil
		}
	}
	if s.settled[strings.ToLower(a.Name)] {
		if hold {
			s.say("activity %q (%d): matches", a.Name, a.ID)
		}
		return false, nil
	}
	reduced, held := a, false
	if hold {
		reduced, held = s.withoutCopyKeys(a)
	}
	acts, err := s.activities()
	if err != nil {
		return false, err
	}
	match := s.match(acts, a.ID, a.Name)
	if match == nil {
		if s.opts.DryRun {
			s.say("activity %q: create, with %d devices", a.Name, len(a.Devices))
			return false, nil
		}
		cp := *reduced
		cp.ID = 0
		id, err := CreateActivity(s.ctx, s.c, &cp, func(line string) { s.say("  %s", line) })
		if err != nil {
			return false, fmt.Errorf("profile: creating activity %q: %w", a.Name, err)
		}
		s.say("activity %q: created as %d", a.Name, id)
		return held, nil
	}
	st, err := ReadActivity(s.ctx, s.c, match.ID)
	if err != nil {
		return false, fmt.Errorf("profile: activity %q: %w", a.Name, err)
	}
	plan := func(a *Activity) ([]ActivityChange, error) {
		cp := *a
		cp.ID = match.ID
		return PlanActivity(&cp, st, s.opts.Prune)
	}
	changes, err := plan(a)
	if err != nil && held {
		// The copies are missing: plan without the keys running them.
		if changes, err = plan(reduced); err == nil {
			pending = true
		}
	}
	if err != nil {
		if s.opts.DryRun {
			// Its devices may be ones the sync would create.
			s.say("activity %q (%d): cannot be planned before the devices are created: %v", a.Name, match.ID, err)
			return false, nil
		}
		return false, fmt.Errorf("profile: activity %q: %w", a.Name, err)
	}
	if len(changes) == 0 {
		if !pending || hold {
			s.say("activity %q (%d): matches", a.Name, match.ID)
		}
		return pending, nil
	}
	s.say("activity %q (%d): %d changes", a.Name, match.ID, len(changes))
	if s.opts.DryRun {
		for _, ch := range changes {
			s.say("  %v", ch)
		}
		return false, nil
	}
	return pending, ApplyActivity(s.ctx, s.c, changes, func(ch ActivityChange) { s.say("  %v", ch) })
}

// extras reports the hub's devices and activities that the config lacks,
// unless they were deleted.
func (s *syncer) extras() error {
	if s.opts.Delete {
		return nil
	}
	devs, acts, err := s.unmatched()
	if err != nil {
		return err
	}
	for _, e := range devs {
		s.say("device %q (%d): not in the configuration, kept", e.Name, e.ID)
	}
	for _, e := range acts {
		s.say("activity %q (%d): not in the configuration, kept", e.Name, e.ID)
	}
	return nil
}
