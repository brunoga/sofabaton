package profile

import (
	"bytes"
	"context"
	"fmt"
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
)

func (k ChangeKind) String() string {
	switch k {
	case Add:
		return "add"
	case Update:
		return "update"
	case Remove:
		return "remove"
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
	}
	return c.Kind.String()
}

// State is what a hub holds for one device: its commands and their signal
// data by command id.
type State struct {
	Variant  wire.Variant
	Device   byte
	Commands []*record.Command
	Data     map[byte][]byte
}

// Read reads a device's state from a hub: its command list, then each
// command's data.
func Read(ctx context.Context, c *hub.Client, device byte) (*State, error) {
	cmds, err := c.Commands(ctx, device)
	if err != nil {
		return nil, err
	}
	st := &State{Variant: c.Variant(), Device: device, Commands: cmds, Data: map[byte][]byte{}}
	for _, cmd := range cmds {
		_, data, err := c.CommandData(ctx, device, cmd.ID)
		if err != nil {
			return nil, err
		}
		st.Data[cmd.ID] = data
	}
	return st, nil
}

// Plan compares a profile with a device's state on the hub and returns the changes that make the hub match
// the profile, updates first, then adds, then (with prune) removes.
// Commands are matched by exact name, then by signal (a rename), then by
// name regardless of case; the hub's commands the profile does not match
// are removed only with prune, and returned as kept otherwise.
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
	for i := range d.Commands {
		claim(i, func(h *record.Command) bool { return bytes.Equal(data[h.ID], signals[i]) })
	}
	for i, c := range d.Commands {
		claim(i, func(h *record.Command) bool { return strings.EqualFold(h.Label, c.Name) })
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
	for _, h := range cmds {
		if claimed[h.ID] {
			continue
		}
		if prune {
			changes = append(changes, Change{Kind: Remove, Command: h})
		} else {
			kept = append(kept, h)
		}
	}
	return changes, kept, nil
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
