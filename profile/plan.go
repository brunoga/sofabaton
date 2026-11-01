package profile

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/brunoga/sofabaton/hub"
	"github.com/brunoga/sofabaton/record"
	"github.com/brunoga/sofabaton/wire"
)

// ChangeKind is what a Change does.
type ChangeKind int

const (
	// Add writes a new command.
	Add ChangeKind = iota + 1
	// Update rewrites a command in place: a new name, a new signal or
	// both. Its id, code and the bindings to it stay.
	Update
	// Remove deletes a command the profile does not have.
	Remove
	// Reorder writes the order of the device's page on the remote.
	Reorder
	// Bind writes a key's binding.
	Bind
	// Sequence writes a power sequence.
	Sequence
	// Inputs writes the device's inputs page.
	Inputs
	// Record rewrites the device's record: its name and brand, its power
	// and input modes. Activities refer to the device by id, so a rename
	// keeps them.
	Record
	// Sync updates the remotes' copy of the device.
	Sync
)

func (k ChangeKind) String() string {
	switch k {
	case Add:
		return "add"
	case Update:
		return "update"
	case Remove:
		return "remove"
	case Reorder:
		return "order"
	case Bind:
		return "button"
	case Sequence:
		return "sequence"
	case Inputs:
		return "inputs"
	case Record:
		return "record"
	case Sync:
		return "sync"
	}
	return fmt.Sprintf("ChangeKind(%d)", int(k))
}

// Change is one step of a plan.
type Change struct {
	Kind ChangeKind
	// Command is the hub's command for Update and Remove; for Add, the
	// command to write (its id the next free one).
	Command *record.Command
	// Name and Data are the command's new name and signal (Add, Update).
	Name string
	Data []byte
	// Renamed and Recoded tell what an Update changes.
	Renamed, Recoded bool
	// Device and Order are the device and its new order (Reorder).
	Device byte
	Order  []record.Place
	// Binding is the key's new binding (Bind); Key names its commands.
	Binding record.Binding
	Key     Key
	// Macro is the new power sequence (Sequence), running Name.
	Macro *record.Macro
	// Page is the new inputs page (Inputs), selected by Names.
	Page  *record.Inputs
	Names []string
	// Entity is the device's new record (Record); Renamed tells whether
	// its name or brand changes, from Name.
	Entity *record.Entity
}

func (c Change) String() string {
	switch c.Kind {
	case Add:
		return fmt.Sprintf("add     %3d %q", c.Command.ID, c.Name)
	case Update:
		var what []string
		if c.Renamed {
			what = append(what, fmt.Sprintf("rename %q -> %q", c.Command.Label, c.Name))
		}
		if c.Recoded {
			what = append(what, "new signal")
		}
		return fmt.Sprintf("update  %3d %s", c.Command.ID, strings.Join(what, ", "))
	case Remove:
		return fmt.Sprintf("remove  %3d %q", c.Command.ID, c.Command.Label)
	case Reorder:
		return fmt.Sprintf("order       %d commands, as the profile lists them", len(c.Order))
	case Bind:
		s := fmt.Sprintf("button      %v -> %q", c.Binding.Button, c.Key.Short)
		if c.Key.Long != "" {
			s += fmt.Sprintf(", long %q", c.Key.Long)
		}
		return s
	case Sequence:
		return fmt.Sprintf("sequence    %s -> %q", c.Macro.Label, c.Name)
	case Inputs:
		if len(c.Names) == 0 {
			return "inputs      none: the device does not switch inputs"
		}
		return fmt.Sprintf("inputs      %q", c.Names)
	case Record:
		t, _ := c.Entity.DeviceTail()
		s := fmt.Sprintf("record      power mode %d style %d, input mode %d", t.PowerMode, t.PowerStyle, t.InputMode)
		if c.Renamed {
			s = fmt.Sprintf("record      rename device %q -> %q (brand %q), power mode %d style %d, input mode %d", c.Name, c.Entity.Name, c.Entity.Brand, t.PowerMode, t.PowerStyle, t.InputMode)
		}
		return s
	case Sync:
		return fmt.Sprintf("sync        device %d to the remotes", c.Device)
	}
	return c.Kind.String()
}

// State is what a hub holds for one device: its commands, their signal
// data by command id, their order on the remote (nil when the device has
// none), the device's key bindings, its sequences, its record and its
// inputs page (nil when it has none).
type State struct {
	Variant  wire.Variant
	Device   byte
	Commands []*record.Command
	Data     map[byte][]byte
	Order    []record.Place
	Keymap   []record.Binding
	Macros   []*record.Macro
	Entity   *record.Entity
	Inputs   *record.Inputs
}

// Read reads a device's state from a hub: its command list, each
// command's data, its order, its keymap, its sequences, its record and its
// inputs.
func Read(ctx context.Context, c *hub.Client, device byte) (*State, error) {
	cmds, err := c.Commands(ctx, device)
	if err != nil {
		return nil, err
	}
	st := &State{Variant: c.Variant(), Device: device, Commands: cmds, Data: map[byte][]byte{}}
	devs, err := c.Devices(ctx)
	if err != nil {
		return nil, err
	}
	for _, d := range devs {
		if d.ID == device {
			st.Entity = d
		}
	}
	if st.Entity == nil {
		return nil, fmt.Errorf("profile: %w: device %d", hub.ErrNotFound, device)
	}
	if st.Inputs, err = c.Inputs(ctx, device); err != nil && !errors.Is(err, hub.ErrNotFound) {
		return nil, err
	}
	if st.Order, err = c.Order(ctx, device); err != nil && !errors.Is(err, hub.ErrNotFound) {
		return nil, err
	}
	if st.Keymap, err = c.Keymap(ctx, device); err != nil && !errors.Is(err, hub.ErrNotFound) {
		return nil, err
	}
	if st.Macros, err = c.Macros(ctx, device); err != nil && !errors.Is(err, hub.ErrNotFound) {
		return nil, err
	}
	for _, cmd := range cmds {
		_, data, err := c.CommandData(ctx, device, cmd.ID)
		if err != nil {
			return nil, err
		}
		st.Data[cmd.ID] = data
	}
	return st, nil
}

// Plan compares a profile with a device's state on the hub and returns the
// changes that make the hub match the profile: command updates, adds and
// (with prune) removes, then the order, the key bindings, the power
// sequences, the inputs page, the device's record (power and input modes)
// and, after any of them, a sync to the remotes. Commands
// are matched by exact name, then by name regardless of case, then by
// signal (a rename: several commands can share one, so names go first); the hub's commands the profile does not match are
// removed only with prune, and returned as kept otherwise, listed after
// the profile's. Keys the profile does not bind are left as they are.
func Plan(d *Device, st *State, prune bool) (changes []Change, kept []*record.Command, err error) {
	cmds, data := st.Commands, st.Data
	if err := d.Validate(); err != nil {
		return nil, nil, err
	}
	signals := make([][]byte, len(d.Commands))
	for i, c := range d.Commands {
		if signals[i], err = c.Signal(d); err != nil {
			return nil, nil, err
		}
	}
	match := make([]*record.Command, len(d.Commands))
	claimed := map[byte]bool{}
	claim := func(i int, ok func(*record.Command) bool) {
		if match[i] != nil {
			return
		}
		for _, h := range cmds {
			if !claimed[h.ID] && ok(h) {
				match[i], claimed[h.ID] = h, true
				return
			}
		}
	}
	for i, c := range d.Commands {
		claim(i, func(h *record.Command) bool { return h.Label == c.Name })
	}
	for i, c := range d.Commands {
		claim(i, func(h *record.Command) bool { return strings.EqualFold(h.Label, c.Name) })
	}
	for i := range d.Commands {
		claim(i, func(h *record.Command) bool { return bytes.Equal(data[h.ID], signals[i]) })
	}

	var adds []Change
	used := append([]*record.Command(nil), cmds...)
	for i, c := range d.Commands {
		h := match[i]
		if h != nil {
			renamed, recoded := h.Label != c.Name, !bytes.Equal(data[h.ID], signals[i])
			if renamed || recoded {
				changes = append(changes, Change{Kind: Update, Command: h, Name: c.Name, Data: signals[i], Renamed: renamed, Recoded: recoded})
			}
			continue
		}
		if c.Data != "" {
			return nil, nil, fmt.Errorf("profile: %q: only commands with a code or a descriptor can be added; data is for keeping existing ones", c.Name)
		}
		id, err := hub.NextCommandID(used)
		if err != nil {
			return nil, nil, err
		}
		nc, err := record.NewCommand(st.Variant)
		if err != nil {
			return nil, nil, err
		}
		nc.Device, nc.ID, nc.Type, nc.Label = st.Device, id, record.ClassIR, c.Name
		used = append(used, nc)
		adds = append(adds, Change{Kind: Add, Command: nc, Name: c.Name, Data: signals[i]})
	}
	changes = append(changes, adds...)
	removed := map[byte]bool{}
	for _, id := range displayOrder(cmds, st.Order) {
		if claimed[id] {
			continue
		}
		var h *record.Command
		for _, c := range cmds {
			if c.ID == id {
				h = c
			}
		}
		if prune {
			changes = append(changes, Change{Kind: Remove, Command: h})
			removed[id] = true
		} else {
			kept = append(kept, h)
		}
	}

	// The profile's commands, at their ids to be, in its order.
	ids := make([]*record.Command, len(d.Commands))
	copy(ids, match)
	for _, a := range adds {
		ids[d.find(a.Name)] = a.Command
	}
	var after []*record.Command // what the hub will hold
	for _, c := range used {
		if !removed[c.ID] {
			after = append(after, c)
		}
	}
	want := make([]byte, 0, len(after))
	for _, c := range ids {
		want = append(want, c.ID)
	}
	for _, c := range kept {
		want = append(want, c.ID)
	}
	if !slices.Equal(want, displayOrder(after, st.Order)) {
		order := make([]record.Place, len(want))
		for i, id := range want {
			order[i] = record.Place{Command: id, Position: byte(i + 1)}
		}
		changes = append(changes, Change{Kind: Reorder, Device: st.Device, Order: order})
	}

	rows := map[record.Button]record.Binding{}
	for _, r := range st.Keymap {
		rows[r.Button] = r
	}
	press := func(name string) record.Press {
		if name == "" {
			return record.Press{}
		}
		c := ids[d.find(name)]
		return record.Press{Device: st.Device, Code: c.Code, Command: c.ID}
	}
	same := func(a, b record.Press) bool { return a.Device == b.Device && a.Command == b.Command }
	var binds []Change
	for name, k := range d.Buttons {
		btn, _ := record.ParseButton(name) // validated
		b := record.Binding{Entity: st.Device, Button: btn, Short: press(k.Short), Long: press(k.Long)}
		if r, ok := rows[btn]; ok && same(r.Short, b.Short) && same(r.Long, b.Long) {
			continue
		}
		binds = append(binds, Change{Kind: Bind, Binding: b, Key: k})
	}
	slices.SortFunc(binds, func(a, b Change) int { return int(a.Binding.Button) - int(b.Binding.Button) })
	changes = append(changes, binds...)

	if p := d.Power; p != nil {
		for _, seq := range []struct {
			id      byte
			label   string
			command string
		}{{record.MacroPowerOn, "POWER_ON", p.On}, {record.MacroPowerOff, "POWER_OFF", p.Off}} {
			pr := press(seq.command)
			step := record.Step{Device: pr.Device, Command: pr.Command, Code: pr.Code, Delay: 0xFF}
			var m *record.Macro
			for _, x := range st.Macros {
				if x.ID == seq.id {
					m = x
				}
			}
			if m != nil && len(m.Steps) == 1 && m.Steps[0].Device == step.Device && m.Steps[0].Command == step.Command {
				continue
			}
			if m == nil {
				if m, err = record.NewMacro(st.Variant, st.Device, seq.id, seq.label); err != nil {
					return nil, nil, err
				}
			} else {
				cp := *m // keeps the hub's label slot, which it insists on
				m = &cp
			}
			m.Steps = []record.Step{step}
			changes = append(changes, Change{Kind: Sequence, Macro: m, Name: d.Commands[d.find(seq.command)].Name})
		}
	}

	if d.Inputs != nil {
		var entries []record.InputEntry
		var names []string
		for i, n := range d.Inputs {
			pr := press(n)
			name := d.Commands[d.find(n)].Name
			entries = append(entries, record.InputEntry{Command: pr.Command, Code: pr.Code, Ordinal: byte(i + 1), Label: name})
			names = append(names, name)
		}
		// A device without inputs gets an empty page, as the app writes.
		mode := record.InputsDirect
		if len(d.Inputs) == 0 {
			mode = record.InputsNone
		}
		in := st.Inputs
		if in == nil || in.Mode != mode || !sameInputs(in.Entries, entries) {
			var page *record.Inputs
			if in != nil {
				cp := *in // keeps the control keys and favorites
				page = &cp
				page.Mode = mode
			} else if page, err = record.NewInputs(st.Variant, st.Device, mode); err != nil {
				return nil, nil, err
			}
			page.Entries = entries
			changes = append(changes, Change{Kind: Inputs, Page: page, Names: names})
		}
	}
	if e := st.Entity; e != nil {
		t, err := e.DeviceTail()
		if err != nil {
			return nil, nil, err
		}
		want := t
		if d.Power != nil {
			want.PowerMode, want.PowerStyle = powerMode, powerStyle
		}
		switch {
		case len(d.Inputs) > 0:
			want.InputMode = record.InputsDirect
		case d.Inputs != nil:
			want.InputMode = record.InputsNoSwitch
		}
		renamed := e.Name != d.Name || (d.Brand != "" && e.Brand != d.Brand)
		if want != t || renamed {
			ne := *e
			ne.Tail = append([]byte(nil), e.Tail...)
			if err := ne.SetDeviceTail(want); err != nil {
				return nil, nil, err
			}
			ne.Name = d.Name
			if d.Brand != "" {
				ne.Brand = d.Brand
			}
			changes = append(changes, Change{Kind: Record, Entity: &ne, Renamed: renamed, Name: e.Name})
		}
	}
	// The remotes keep their own copy of the device and do not see edits
	// until it is synced (our X2's remote kept the old names and order).
	if len(changes) > 0 {
		changes = append(changes, Change{Kind: Sync, Device: st.Device})
	}
	return changes, kept, nil
}

// The tail's power fields once the app has set a device's power up.
const (
	powerMode  = 1
	powerStyle = 3
)

// sameInputs compares input lists by command and position: the app labels
// inputs with its catalog's names ("Hdmi1"), and those can stay.
func sameInputs(a, b []record.InputEntry) bool {
	return slices.EqualFunc(a, b, func(x, y record.InputEntry) bool {
		return x.Command == y.Command && x.Ordinal == y.Ordinal
	})
}

// Apply carries out a plan on the hub, calling done after each change.
func Apply(ctx context.Context, c *hub.Client, changes []Change, done func(Change)) error {
	for _, ch := range changes {
		var err error
		switch ch.Kind {
		case Add:
			err = c.WriteCommand(ctx, ch.Command, ch.Data)
		case Update:
			cmd := *ch.Command
			cmd.Label = ch.Name
			err = c.WriteCommand(ctx, &cmd, ch.Data)
		case Remove:
			err = c.DeleteCommand(ctx, ch.Command.Device, ch.Command.ID)
		case Reorder:
			err = c.WriteOrder(ctx, ch.Device, ch.Order)
		case Bind:
			err = c.WriteButton(ctx, ch.Binding)
		case Sequence:
			err = c.WriteMacro(ctx, ch.Macro)
		case Inputs:
			err = c.WriteInputs(ctx, ch.Page)
		case Record:
			err = c.UpdateDevice(ctx, ch.Entity)
		case Sync:
			err = c.Sync(ctx, ch.Device)
		default:
			err = fmt.Errorf("profile: unknown change %v", ch.Kind)
		}
		if err != nil {
			return fmt.Errorf("profile: %v: %w", ch, err)
		}
		if done != nil {
			done(ch)
		}
	}
	return nil
}
