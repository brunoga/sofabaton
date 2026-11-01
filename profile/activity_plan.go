package profile

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/brunoga/sofabaton/hub"
	"github.com/brunoga/sofabaton/record"
	"github.com/brunoga/sofabaton/wire"
)

// ActivityChangeKind is what an ActivityChange does.
type ActivityChangeKind int

const (
	// ActMembers confirms the activity's devices to the hub, in order,
	// before its power macros change them.
	ActMembers ActivityChangeKind = iota + 1
	// ActMacro rewrites one of the activity's macros in place: a
	// power macro or one of its own.
	ActMacro
	// ActNewMacro adds a macro; the hub gives it its id.
	ActNewMacro
	// ActRemoveMacro deletes a macro the profile does not have.
	ActRemoveMacro
	// ActOrder writes the order of the remote's list of macros.
	ActOrder
	// ActBind writes a key's binding.
	ActBind
	// ActSync updates the remotes' copy of the activity.
	ActSync
	// ActRecord rewrites the activity's record: its name, or the record
	// as the hub holds it after its members change, as the app does.
	ActRecord
)

// ActivityChange is one step of an activity's plan.
type ActivityChange struct {
	Kind     ActivityChangeKind
	Activity byte
	// Members are the devices to confirm (ActMembers), by id and name.
	Members []byte
	Names   []string
	// Macro is the macro to write (ActMacro, ActNewMacro with id 0)
	// or delete (ActRemoveMacro).
	Macro *record.Macro
	// Order is the new list (ActOrder): macros by name, so that new
	// ones get the ids the hub gives them, and the rest by id.
	Order []OrderEntry
	// Binding is the key's new binding (ActBind). A press of a new
	// macro names it in Key and gets its id when the change is applied.
	Binding record.Binding
	Key     Key
	// Entity is the activity's new record (ActRecord); Renamed tells
	// whether its name changes, from Name. For a copy of a device's
	// macro, Name is the device's.
	Entity  *record.Entity
	Renamed bool
	Name    string

	shortSeq, longSeq string // new macros of Binding to resolve, by macroKey
}

// OrderEntry is one entry of an activity's list: one of its own macros
// by name, a new copy of a device's macro (by its key), or anything else
// by id.
type OrderEntry struct {
	Macro string
	ID    byte

	Copy string
}

func (c ActivityChange) String() string {
	switch c.Kind {
	case ActMembers:
		return fmt.Sprintf("members     %s", strings.Join(c.Names, ", "))
	case ActMacro:
		return fmt.Sprintf("macro       rewrite %s, %d steps%s", c.Macro.Label, len(c.Macro.Steps), c.copyOf())
	case ActNewMacro:
		return fmt.Sprintf("macro       add %q, %d steps%s", c.Macro.Label, len(c.Macro.Steps), c.copyOf())
	case ActRemoveMacro:
		return fmt.Sprintf("macro       remove %q%s", c.Macro.Label, c.copyOf())
	case ActOrder:
		return fmt.Sprintf("order       %d macros and favorites", len(c.Order))
	case ActBind:
		s := fmt.Sprintf("button      %v -> %q", c.Binding.Button, c.Key.Short)
		if c.Key.Long != "" {
			s += fmt.Sprintf(", long %q", c.Key.Long)
		}
		return s
	case ActSync:
		return fmt.Sprintf("sync        activity %d to the remotes", c.Activity)
	case ActRecord:
		if c.Renamed {
			return fmt.Sprintf("record      rename activity %q -> %q", c.Name, c.Entity.Name)
		}
		return "record      rewrite the activity's record, as the app does after a member change"
	}
	return fmt.Sprintf("ActivityChangeKind(%d)", int(c.Kind))
}

// copyOf names the device of a copy of a device's macro.
func (c ActivityChange) copyOf() string {
	if c.Macro == nil || c.Macro.Origin == 0 {
		return ""
	}
	if c.Name != "" {
		return fmt.Sprintf(", %s's", c.Name)
	}
	return fmt.Sprintf(", device %d's", c.Macro.Origin)
}

// The hub's keys for the steps that run a device's power macros and
// switch it to its input.
const (
	stepPowerOn  = record.MacroPowerOn
	stepPowerOff = record.MacroPowerOff
	stepInput    = macroInput
)

// resolver turns an activity profile's names into the hub's ids.
type resolver struct {
	st *ActivityState
}

func (r resolver) device(name string) (byte, error) {
	for id, n := range r.st.Names {
		if !record.IsActivityID(id) && strings.EqualFold(n, name) {
			return id, nil
		}
	}
	return 0, fmt.Errorf("profile: no device %q on the hub", name)
}

func (r resolver) command(dev byte, name string) (*record.Command, error) {
	cmds := r.st.Commands[dev]
	for _, c := range cmds {
		if c.Label == name {
			return c, nil
		}
	}
	for _, c := range cmds {
		if strings.EqualFold(c.Label, name) {
			return c, nil
		}
	}
	if n, err := strconv.ParseUint(strings.TrimPrefix(name, "#"), 10, 8); err == nil && strings.HasPrefix(name, "#") {
		for _, c := range cmds {
			if c.ID == byte(n) {
				return c, nil
			}
		}
	}
	return nil, fmt.Errorf("profile: %s has no command %q", r.st.Names[dev], name)
}

// ordinal is the ordinal of the device's input selected by the command
// named input, or 0 for none.
func (r resolver) ordinal(dev byte, input string) (byte, error) {
	if input == "" {
		return 0, nil
	}
	cmd, err := r.command(dev, input)
	if err != nil {
		return 0, err
	}
	if in := r.st.Inputs[dev]; in != nil {
		for _, e := range in.Entries {
			if e.Command == cmd.ID {
				return e.Ordinal, nil
			}
		}
	}
	return 0, fmt.Errorf("profile: %q is not one of %s's inputs", input, r.st.Names[dev])
}

func waitStep(seconds byte) record.Step {
	return record.Step{Device: 0xFF, Command: 0xFF, Code: [record.CodeLen]byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}, Duration: 0xFF, Delay: seconds}
}

// steps turns a profile's steps into the hub's, with members' input
// ordinals for input steps.
func (r resolver) steps(steps []Step, inputs map[byte]byte) ([]record.Step, error) {
	var out []record.Step
	for _, s := range steps {
		if s.Kind == StepWait {
			out = append(out, waitStep(s.Seconds))
			continue
		}
		dev, err := r.device(s.Device)
		if err != nil {
			return nil, err
		}
		switch s.Kind {
		case StepPowerOn:
			out = append(out, record.Step{Device: dev, Command: stepPowerOn, Delay: 0xFF})
		case StepPowerOff:
			out = append(out, record.Step{Device: dev, Command: stepPowerOff, Delay: 0xFF})
		case StepInput:
			out = append(out, record.Step{Device: dev, Command: stepInput, Duration: inputs[dev], Delay: 0xFF})
		default:
			cmd, err := r.command(dev, s.Command)
			if err != nil {
				return nil, err
			}
			step := hub.StepFor(cmd)
			step.Duration = s.Hold
			out = append(out, step)
		}
	}
	return out, nil
}

// sameSteps compares macros, but for the duration of the steps that
// run a device's power macros, which the hub sets itself (1 for IR
// devices, 0 for Bluetooth ones, whatever the app wrote).
func sameSteps(a, b []record.Step) bool {
	return slices.EqualFunc(a, b, func(x, y record.Step) bool {
		if x.Command == stepPowerOn || x.Command == stepPowerOff {
			x.Duration, y.Duration = 0, 0
		}
		return x == y
	})
}

// defaultPower is the power macro the app writes for members: each
// device's power macro a second apart and, turning on, then each
// device's input.
func defaultPower(members []byte, inputs map[byte]byte, on bool) []record.Step {
	var out []record.Step
	key := byte(stepPowerOff)
	if on {
		key = stepPowerOn
	}
	for _, d := range members {
		out = append(out, record.Step{Device: d, Command: key, Delay: 0xFF}, waitStep(1))
	}
	if on {
		for _, d := range members {
			out = append(out, record.Step{Device: d, Command: stepInput, Duration: inputs[d], Delay: 0xFF}, waitStep(1))
		}
	}
	return out
}

// PlanActivity compares an activity's profile with its state on the hub
// and returns the changes that make the hub match: its members and power
// macros, its own macros (matched by name; removed only with
// prune), their order on the remote, its keys, a sync and its record.
// Without power_on or power_off, a new activity, or one whose devices
// change, gets the app's: each device's power macro and input, a
// second apart; others keep theirs, with the devices' inputs. Keys the profile does not bind are
// left as they are.
func PlanActivity(a *Activity, st *ActivityState, prune bool) ([]ActivityChange, error) {
	if err := a.Validate(); err != nil {
		return nil, err
	}
	act := st.Entity.ID
	r := resolver{st: st}
	var changes []ActivityChange

	var members []byte
	var names []string
	inputs := map[byte]byte{}
	for _, m := range a.Devices {
		id, err := r.device(m.Device)
		if err != nil {
			return nil, err
		}
		if inputs[id], err = r.ordinal(id, m.Input); err != nil {
			return nil, err
		}
		members = append(members, id)
		names = append(names, st.Names[id])
	}
	var have []byte
	for _, m := range st.Members {
		have = append(have, m.ID)
	}
	membersChange := !slices.Equal(members, have)
	if membersChange {
		changes = append(changes, ActivityChange{Kind: ActMembers, Activity: act, Members: members, Names: names})
	}

	macros := map[byte]*record.Macro{}
	for _, m := range st.Macros {
		macros[m.ID] = m
	}
	rewrite := func(id byte, label string, steps []record.Step) error {
		m := macros[id]
		if m != nil && sameSteps(m.Steps, steps) {
			return nil
		}
		if m != nil {
			cp := *m // keeps the hub's label slot, which it insists on
			m = &cp
		} else {
			var err error
			if m, err = record.NewMacro(st.variant(), act, id, label); err != nil {
				return err
			}
		}
		m.Steps = steps
		changes = append(changes, ActivityChange{Kind: ActMacro, Activity: act, Macro: m})
		return nil
	}
	for _, p := range []struct {
		id    byte
		label string
		steps []Step
		on    bool
	}{{record.MacroPowerOn, "POWER_ON", a.PowerOn, true}, {record.MacroPowerOff, "POWER_OFF", a.PowerOff, false}} {
		steps := defaultPower(members, inputs, p.on)
		if p.steps == nil && !membersChange && !st.fresh {
			// The hub's, as long as the devices are the same, with the
			// members' inputs.
			m := macros[p.id]
			if m == nil {
				continue
			}
			steps = slices.Clone(m.Steps)
			for i, s := range steps {
				if s.Command == stepInput && !s.IsDelay() {
					steps[i].Duration = inputs[s.Device]
				}
			}
		}
		if p.steps != nil {
			var err error
			if steps, err = r.steps(p.steps, inputs); err != nil {
				return nil, fmt.Errorf("%w (%s)", err, strings.ToLower(p.label))
			}
		}
		if err := rewrite(p.id, p.label, steps); err != nil {
			return nil, err
		}
	}

	// The activity's own macros, by name, and the copies of its devices'
	// (by macroKey), which their profiles keep: the copies of a device
	// that leaves go, a device that joins brings its macros.
	own := map[string]*record.Macro{}
	copies := map[string]*record.Macro{}
	var removed []byte
	for _, m := range st.Macros {
		switch {
		case m.ID == record.MacroPowerOn || m.ID == record.MacroPowerOff:
		case m.Origin == 0:
			own[strings.ToLower(m.Label)] = m
		case !slices.Contains(members, m.Origin):
			changes = append(changes, ActivityChange{Kind: ActRemoveMacro, Activity: act, Macro: m, Name: st.Names[m.Origin]})
			removed = append(removed, m.ID)
		default:
			copies[macroKey(m)] = m
		}
	}
	reorder := len(removed) > 0
	var newCopies []string
	for _, d := range members {
		for _, src := range st.DeviceMacros[d] {
			key := macroKey(src)
			if copies[key] != nil || slices.Contains(newCopies, key) {
				continue
			}
			m, err := record.NewMacro(st.variant(), act, 0, src.Label)
			if err != nil {
				return nil, err
			}
			m.Origin, m.Steps = d, slices.Clone(src.Steps)
			changes = append(changes, ActivityChange{Kind: ActNewMacro, Activity: act, Macro: m, Name: st.Names[d]})
			newCopies = append(newCopies, key)
			reorder = true
		}
	}
	wanted := map[string]bool{}
	for _, s := range a.Macros {
		steps, err := r.steps(s.Steps, inputs)
		if err != nil {
			return nil, fmt.Errorf("%w (macro %s)", err, s.Name)
		}
		wanted[strings.ToLower(s.Name)] = true
		if m := own[strings.ToLower(s.Name)]; m != nil {
			if err := rewrite(m.ID, m.Label, steps); err != nil {
				return nil, err
			}
			continue
		}
		m, err := record.NewMacro(st.variant(), act, 0, s.Name)
		if err != nil {
			return nil, err
		}
		m.Steps = steps
		changes = append(changes, ActivityChange{Kind: ActNewMacro, Activity: act, Macro: m})
		reorder = true
	}
	for name, m := range own {
		if prune && !wanted[name] {
			changes = append(changes, ActivityChange{Kind: ActRemoveMacro, Activity: act, Macro: m})
			removed = append(removed, m.ID)
			reorder = true
		}
	}

	// The remote's list: the profile's macros in its order, then what
	// else the list has (favorites, macros kept), in their order.
	seqID := map[byte]string{}
	for name, m := range own {
		seqID[m.ID] = name
	}
	var order []OrderEntry
	for _, s := range a.Macros {
		order = append(order, OrderEntry{Macro: s.Name})
	}
	var tail []OrderEntry // new copies, after the rest
	for _, k := range newCopies {
		tail = append(tail, OrderEntry{Copy: k})
	}
	placed := slices.Clone(st.Order)
	slices.SortStableFunc(placed, func(x, y record.Place) int { return int(x.Position) - int(y.Position) })
	var current []OrderEntry
	for _, p := range placed {
		if !p.Placed() || slices.Contains(removed, p.Command) {
			continue
		}
		if name, ok := seqID[p.Command]; ok {
			current = append(current, OrderEntry{Macro: name})
			if wanted[name] {
				continue
			}
		} else {
			current = append(current, OrderEntry{ID: p.Command})
		}
		order = append(order, current[len(current)-1])
	}
	if len(st.Order) == 0 && len(tail) > 0 {
		// Without a list the remote shows the macros by id: keep them
		// first.
		for _, m := range st.Macros {
			if m.ID != record.MacroPowerOn && m.ID != record.MacroPowerOff && !slices.Contains(removed, m.ID) && (m.Origin != 0 || !wanted[strings.ToLower(m.Label)]) {
				order = append(order, OrderEntry{ID: m.ID})
			}
		}
	}
	order = append(order, tail...)
	if reorder || len(st.Order) > 0 && !slices.EqualFunc(order, current, func(x, y OrderEntry) bool {
		return strings.EqualFold(x.Macro, y.Macro) && x.ID == y.ID
	}) {
		if len(order) > 0 {
			changes = append(changes, ActivityChange{Kind: ActOrder, Activity: act, Order: order})
		}
	}

	// Keys: "Device: Command", or a macro's name.
	bound := map[record.Button]record.Binding{}
	for _, b := range st.Keymap {
		bound[b.Button] = b
	}
	press := func(s string) (record.Press, string, error) {
		if s == "" {
			return record.Press{}, "", nil
		}
		if wanted[strings.ToLower(s)] || own[strings.ToLower(s)] != nil {
			if m := own[strings.ToLower(s)]; m != nil {
				return record.Press{Device: act, Command: m.ID}, "", nil
			}
			return record.Press{Device: act}, strings.ToLower(s), nil // a new macro
		}
		devName, cmdName, _ := strings.Cut(s, ": ")
		dev, err := r.device(devName)
		if err != nil {
			return record.Press{}, "", err
		}
		cmd, err := r.command(dev, cmdName)
		if err != nil {
			// One of the device's macros, by its copy here.
			key := copyKey(dev, cmdName)
			if m := copies[key]; m != nil {
				return record.Press{Device: act, Command: m.ID}, "", nil
			}
			if slices.Contains(newCopies, key) {
				return record.Press{Device: act}, key, nil
			}
			return record.Press{}, "", fmt.Errorf("%w, nor a macro of it", err)
		}
		return record.Press{Device: dev, Command: cmd.ID}, "", nil
	}
	var buttons []record.Button
	keys := map[record.Button]Key{}
	for name, k := range a.Buttons {
		b, err := record.ParseButton(name)
		if err != nil {
			return nil, err
		}
		buttons = append(buttons, b)
		keys[b] = k
	}
	slices.Sort(buttons)
	for _, b := range buttons {
		k := keys[b]
		short, shortSeq, err := press(k.Short)
		if err != nil {
			return nil, fmt.Errorf("%w (button %v)", err, b)
		}
		long, longSeq, err := press(k.Long)
		if err != nil {
			return nil, fmt.Errorf("%w (button %v)", err, b)
		}
		old, ok := bound[b]
		same := func(x, y record.Press) bool { return x.Device == y.Device && x.Command == y.Command }
		if ok && shortSeq == "" && longSeq == "" && same(old.Short, short) && same(old.Long, long) {
			continue
		}
		changes = append(changes, ActivityChange{Kind: ActBind, Activity: act, Key: k,
			Binding: record.Binding{Entity: act, Button: b, Short: short, Long: long}, shortSeq: shortSeq, longSeq: longSeq})
	}

	renamed := st.Entity.Name != a.Name
	if len(changes) > 0 {
		changes = append(changes, ActivityChange{Kind: ActSync, Activity: act})
	}
	if renamed || membersChange {
		e := *st.Entity
		e.Tail = append([]byte(nil), st.Entity.Tail...)
		e.Name = a.Name
		changes = append(changes, ActivityChange{Kind: ActRecord, Activity: act, Entity: &e, Renamed: renamed, Name: st.Entity.Name})
	}
	return changes, nil
}

// variant is the hub's model line, from the records read.
func (st *ActivityState) variant() wire.Variant {
	if st.Entity != nil {
		return st.Entity.Variant()
	}
	return wire.X2
}

// ApplyActivity carries out an activity's plan on the hub, calling done
// after each change.
func ApplyActivity(ctx context.Context, c *hub.Client, changes []ActivityChange, done func(ActivityChange)) error {
	ap := activityApplier{ids: map[string]byte{}}
	for _, ch := range changes {
		if err := ap.apply(ctx, c, ch); err != nil {
			return fmt.Errorf("profile: %v: %w", ch, err)
		}
		if done != nil {
			done(ch)
		}
	}
	return nil
}

// activityApplier carries out activity changes, keeping the ids the hub
// gives new macros for the changes after them.
type activityApplier struct {
	ids map[string]byte // new macros by macroKey
}

// macroKey names a macro of an activity: its own by label, a copy of a
// device's macro by the device and the label.
func macroKey(m *record.Macro) string {
	if m.Origin != 0 {
		return copyKey(m.Origin, m.Label)
	}
	return strings.ToLower(m.Label)
}

func copyKey(device byte, label string) string {
	return fmt.Sprintf("%d\x00%s", device, strings.ToLower(label))
}

func (ap activityApplier) apply(ctx context.Context, c *hub.Client, ch ActivityChange) error {
	resolve := func(p *record.Press, key string) error {
		if key == "" {
			return nil
		}
		id, ok := ap.ids[key]
		if !ok {
			return fmt.Errorf("profile: macro %q was not created", key)
		}
		p.Command = id
		return nil
	}
	switch ch.Kind {
	case ActMembers:
		for _, d := range ch.Members {
			if err := c.ConfirmMember(ctx, d); err != nil {
				return err
			}
		}
		return nil
	case ActMacro:
		return c.WriteMacro(ctx, ch.Macro)
	case ActNewMacro:
		id, err := c.CreateMacro(ctx, ch.Macro)
		if err == nil {
			ap.ids[macroKey(ch.Macro)] = id
		}
		return err
	case ActRemoveMacro:
		return c.DeleteMacro(ctx, ch.Activity, ch.Macro.ID)
	case ActOrder:
		var places []record.Place
		for i, e := range ch.Order {
			id := e.ID
			switch {
			case e.Copy != "":
				var ok bool
				if id, ok = ap.ids[e.Copy]; !ok {
					return fmt.Errorf("profile: macro %q was not created", e.Copy)
				}
			case e.Macro != "":
				var ok bool
				if id, ok = ap.ids[strings.ToLower(e.Macro)]; !ok {
					var err error
					if id, err = macroID(ctx, c, ch.Activity, e.Macro); err != nil {
						return err
					}
				}
			}
			places = append(places, record.Place{Command: id, Position: byte(i + 1)})
		}
		return c.WriteOrder(ctx, ch.Activity, places)
	case ActBind:
		b := ch.Binding
		if err := resolve(&b.Short, ch.shortSeq); err != nil {
			return err
		}
		if err := resolve(&b.Long, ch.longSeq); err != nil {
			return err
		}
		return c.WriteButton(ctx, b)
	case ActSync:
		return c.Sync(ctx, ch.Activity)
	case ActRecord:
		// The record as the hub holds it now (its power state moves),
		// with the profile's name.
		e := ch.Entity
		if acts, err := c.Activities(ctx); err == nil {
			for _, a := range acts {
				if a.ID == ch.Activity {
					cp := *a
					cp.Tail = append([]byte(nil), a.Tail...)
					cp.Name = ch.Entity.Name
					e = &cp
				}
			}
		}
		return c.UpdateActivity(ctx, e)
	}
	return fmt.Errorf("profile: unknown change %v", ch.Kind)
}

// macroID looks up the id of an activity's own macro by name.
func macroID(ctx context.Context, c *hub.Client, activity byte, name string) (byte, error) {
	macros, err := c.Macros(ctx, activity)
	if err != nil {
		return 0, err
	}
	for _, m := range macros {
		if m.Origin == 0 && strings.EqualFold(m.Label, name) {
			return m.ID, nil
		}
	}
	return 0, fmt.Errorf("profile: activity %d has no macro %q", activity, name)
}

// CreateActivity makes a new activity on the hub from a profile, in the
// app's order: its devices, the record, its power macros, its keys, a
// sync; then its own macros and their order. done is called after
// each step. It returns the new activity's id; if a step after the create
// fails, the activity is left on the hub for the caller to delete.
func CreateActivity(ctx context.Context, c *hub.Client, a *Activity, done func(string)) (byte, error) {
	if err := a.Validate(); err != nil {
		return 0, err
	}
	report := func(s string) {
		if done != nil {
			done(s)
		}
	}
	// Check every name before creating anything.
	st, _, err := readDevices(ctx, c)
	if err != nil {
		return 0, err
	}
	e, err := record.NewActivity(c.Variant(), a.Name, newActivityColor)
	if err != nil {
		return 0, err
	}
	st.Entity = e
	plan, err := PlanActivity(a, st, false)
	if err != nil {
		return 0, err
	}
	var members ActivityChange
	for _, ch := range plan {
		if ch.Kind == ActMembers {
			members = ch
		}
	}
	for _, d := range members.Members {
		if err := c.ConfirmMember(ctx, d); err != nil {
			return 0, err
		}
	}
	report(fmt.Sprintf("members     %s", strings.Join(members.Names, ", ")))
	id, err := c.CreateActivity(ctx, e)
	if err != nil {
		return 0, err
	}
	report(fmt.Sprintf("created activity %d %q", id, a.Name))
	if st, err = ReadActivity(ctx, c, id); err != nil {
		return id, err
	}
	st.fresh = true
	st.Members = nil
	for _, d := range members.Members {
		st.Members = append(st.Members, &record.Entity{ID: d})
	}
	if plan, err = PlanActivity(a, st, false); err != nil {
		return id, err
	}
	return id, ApplyActivity(ctx, c, plan, func(ch ActivityChange) { report(ch.String()) })
}

// newActivityColor is the color new activities get (palette 1-12).
const newActivityColor = 1
