package dump

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/brunoga/sofabaton/hub"
	"github.com/brunoga/sofabaton/hubsim"
	"github.com/brunoga/sofabaton/record"
	"github.com/brunoga/sofabaton/wire"
)

// ErrNoData is returned by Model for a dump without the commands'
// signals (made before sbx 0.2), which cannot be restored.
var ErrNoData = errors.New("dump: the dump has no command data (dumps from before sbx 0.2 lack it): dump the hub again")

// Model decodes the dump's raw records into the configuration of a
// simulated hub.
func (d *File) Model() (*hubsim.Model, error) {
	raw, err := hex.DecodeString(d.Info.Raw)
	if err != nil {
		return nil, fmt.Errorf("dump: info: %w", err)
	}
	info, err := record.ParseHubInfo(raw)
	if err != nil {
		return nil, fmt.Errorf("dump: info: %w", err)
	}
	v, err := wire.VariantFromModel(info.Model)
	if err != nil {
		return nil, fmt.Errorf("dump: %w", err)
	}
	m := &hubsim.Model{
		Info:     info,
		Commands: map[byte][]*record.Command{},
		Data:     map[byte]map[byte][]byte{},
		Keymaps:  map[byte][]record.Binding{},
		Macros:   map[byte][]*record.Macro{},
		Inputs:   map[byte]*record.Inputs{},
		Members:  map[byte][]byte{},
		Orders:   map[byte][]record.Place{},
	}
	if m.Devices, err = entityList(v, d.Devices); err != nil {
		return nil, fmt.Errorf("dump: devices: %w", err)
	}
	if m.Activities, err = entityList(v, d.Activities); err != nil {
		return nil, fmt.Errorf("dump: activities: %w", err)
	}
	for _, e := range d.Entities {
		if err := e.model(v, m); err != nil {
			return nil, fmt.Errorf("dump: %s (%d): %w", e.Name, e.ID, err)
		}
	}
	return m, nil
}

func (e *Entity) model(v wire.Variant, m *hubsim.Model) error {
	id := e.ID
	if b, ok, err := bodies(e.Commands); err != nil {
		return fmt.Errorf("commands: %w", err)
	} else if ok {
		cmds, err := record.ParseCommandList(v, single(b))
		if err != nil {
			return fmt.Errorf("commands: %w", err)
		}
		m.Commands[id] = cmds
		if len(cmds) > 0 && e.Data == nil {
			return ErrNoData
		}
		m.Data[id] = map[byte][]byte{}
		for _, r := range e.Data {
			b, ok, err := bodies(r)
			if err != nil || !ok {
				return fmt.Errorf("%s: %v", r.Request, errOr(err, r.Error))
			}
			_, c, data, err := record.ParseCommandWrite(v, single(b))
			if err != nil {
				return fmt.Errorf("%s: %w", r.Request, err)
			}
			m.Data[id][c.ID] = data
		}
	}
	if b, ok, err := bodies(e.Order); err != nil {
		return fmt.Errorf("order: %w", err)
	} else if ok {
		if _, m.Orders[id], err = record.ParseOrder(single(b)); err != nil {
			return fmt.Errorf("order: %w", err)
		}
	}
	if b, ok, err := bodies(e.Keymap); err != nil {
		return fmt.Errorf("keymap: %w", err)
	} else if ok {
		if m.Keymaps[id], err = record.ParseKeymap(single(b)); err != nil {
			return fmt.Errorf("keymap: %w", err)
		}
	}
	if b, ok, err := bodies(e.Macros); err != nil {
		return fmt.Errorf("macros: %w", err)
	} else if ok {
		for _, body := range b {
			mac, err := record.ParseMacro(v, body)
			if err != nil {
				return fmt.Errorf("macros: %w", err)
			}
			m.Macros[id] = append(m.Macros[id], mac)
		}
	}
	if b, ok, err := bodies(e.Inputs); err != nil {
		return fmt.Errorf("inputs: %w", err)
	} else if ok {
		if m.Inputs[id], err = record.ParseInputs(v, single(b)); err != nil {
			return fmt.Errorf("inputs: %w", err)
		}
	}
	if b, ok, err := bodies(e.Members); err != nil {
		return fmt.Errorf("members: %w", err)
	} else if ok {
		ents, err := entities(v, b)
		if err != nil {
			return fmt.Errorf("members: %w", err)
		}
		for _, d := range ents {
			m.Members[id] = append(m.Members[id], d.ID)
		}
	}
	return nil
}

// Open returns a client of a simulated hub that serves the dump, for
// reading it as a hub (profile.Read, profile.ReadActivity). Writes change
// only the simulation.
func (d *File) Open(ctx context.Context) (*hub.Client, error) {
	m, err := d.Model()
	if err != nil {
		return nil, err
	}
	return hub.Open(ctx, hubsim.Pipe(hubsim.FromModel(m)), hub.Options{})
}

func entityList(v wire.Variant, r *Reply) ([]*record.Entity, error) {
	b, _, err := bodies(r)
	if err != nil {
		return nil, err
	}
	return entities(v, b)
}

// bodies decodes a reply's raw records; ok is false for a reply the hub
// had nothing for (an error, or no reply at all).
func bodies(r *Reply) (out [][]byte, ok bool, err error) {
	if r == nil || len(r.Raw) == 0 {
		return nil, false, nil
	}
	for _, s := range r.Raw {
		b, err := hex.DecodeString(s)
		if err != nil {
			return nil, false, err
		}
		out = append(out, b)
	}
	return out, true, nil
}

func errOr(err error, msg string) any {
	if err != nil {
		return err
	}
	return msg
}
