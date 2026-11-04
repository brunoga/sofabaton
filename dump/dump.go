// Package dump reads a hub's whole configuration into a lossless backup,
// and serves a backup back as a simulated hub, which the readers of
// package profile read as they read a hub (see Open).
//
// A dump keeps every reply's raw record bodies (hex) next to a decoded
// view, so it keeps even the bytes no decoder understands.
package dump

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/brunoga/sofabaton/hub"
	"github.com/brunoga/sofabaton/record"
	"github.com/brunoga/sofabaton/session"
	"github.com/brunoga/sofabaton/wire"
)

// File is a hub's dump.
type File struct {
	Tool       string    `json:"tool"`
	Info       Info      `json:"info"`
	Devices    *Reply    `json:"devices"`
	Activities *Reply    `json:"activities"`
	Entities   []*Entity `json:"entities"`
}

// Info is the hub's identity.
type Info struct {
	Name     string `json:"name"`
	Model    string `json:"model"`
	Firmware int    `json:"firmware"`
	Batch    string `json:"batch"`
	ID       string `json:"id"`
	Raw      string `json:"raw"`
}

// Reply is one request's reply: raw records and the decoded view, or the
// error the request failed with.
type Reply struct {
	Request string   `json:"request"`
	Raw     []string `json:"raw,omitempty"`
	Decoded any      `json:"decoded,omitempty"`
	Error   string   `json:"error,omitempty"`
}

// Entity is what the hub holds for a device or activity. Data has the
// replies with each command's signal, and Order the remote's list (a
// device's commands, an activity's macros and favorites); dumps made
// before sbx 0.2 have neither.
type Entity struct {
	ID       byte     `json:"id"`
	Name     string   `json:"name"`
	Commands *Reply   `json:"commands,omitempty"`
	Data     []*Reply `json:"data,omitempty"`
	Order    *Reply   `json:"order,omitempty"`
	Keymap   *Reply   `json:"keymap,omitempty"`
	Macros   *Reply   `json:"macros,omitempty"`
	Inputs   *Reply   `json:"inputs,omitempty"`
	Members  *Reply   `json:"members,omitempty"`
}

// Read dumps the hub c is connected to. A request the hub refuses is kept
// in the dump, with its error, and reported to log. Read stops at the
// first other failure (a lost connection, a hub that stopped answering),
// and the dump lacks what was not read: ReadProgress reports it.
func Read(ctx context.Context, c *hub.Client, tool string, log io.Writer) *File {
	d, _ := ReadProgress(ctx, c, tool, log, nil)
	return d
}

// ReadProgress is Read, returning the failure that stopped it, and calling
// progress (if not nil) before each device
// and activity with how many are done, how many there are, and the one
// being read; reading a real hub takes about two seconds for each. ctx
// cancels it between them, with ctx's error.
func ReadProgress(ctx context.Context, c *hub.Client, tool string, log io.Writer, progress func(done, total int, name string)) (*File, error) {
	return read(ctx, c, tool, log, progress, nil, nil)
}

// Refresh is ReadProgress for a hub read before into old: a command whose
// record (id, label, type and code) is as old has it keeps old's signal
// instead of reading it again. Reading signals is most of a dump's time
// (19 of 33 s on our X2). old must be a dump of the same hub; a nil old
// reads everything.
//
// The hub keeps a command's record when its signal is written again under
// the same label (on our X2, 2026-11-02): a signal can change without its
// record. So old must hold only signals known to be current, such as a
// dump made before writes, without the devices written to (drop their
// entities' Data).
func Refresh(ctx context.Context, c *hub.Client, old *File, tool string, log io.Writer, progress func(done, total int, name string)) (*File, error) {
	if old != nil {
		i := c.Info()
		if old.Info.ID != hex.EncodeToString(i.ID[:]) {
			return nil, fmt.Errorf("dump: refresh: the old dump is of hub %s, not %x", old.Info.ID, i.ID)
		}
	}
	return read(ctx, c, tool, log, progress, old, nil)
}

// RefreshSome is Refresh for a hub that changed since old was read only in
// the devices and activities stale picks, by id: as after writing to them.
// The others are taken from old whole, without reading them; the lists
// of devices and activities are read again.
func RefreshSome(ctx context.Context, c *hub.Client, old *File, stale func(id byte) bool, tool string, log io.Writer, progress func(done, total int, name string)) (*File, error) {
	if old == nil {
		return nil, fmt.Errorf("dump: refresh: no old dump")
	}
	i := c.Info()
	if old.Info.ID != hex.EncodeToString(i.ID[:]) {
		return nil, fmt.Errorf("dump: refresh: the old dump is of hub %s, not %x", old.Info.ID, i.ID)
	}
	return read(ctx, c, tool, log, progress, old, stale)
}

func read(ctx context.Context, c *hub.Client, tool string, log io.Writer, progress func(done, total int, name string), old *File, stale func(id byte) bool) (*File, error) {
	i := c.Info()
	d := &File{
		Tool: tool,
		Info: Info{Name: i.Name, Model: i.Variant.String(), Firmware: int(i.Firmware),
			Batch: i.BatchDate(), ID: hex.EncodeToString(i.ID[:]), Raw: hex.EncodeToString(i.AppendPayload(nil))},
	}
	v := c.Variant()
	r := &reader{ctx: ctx, c: c, log: log}
	d.Devices = r.reply("devices", wire.Frame{Family: wire.FamReqDevices}, wire.FamDeviceRow,
		func(bodies [][]byte) (any, error) { return entities(v, bodies) })
	d.Activities = r.reply("activities", wire.Frame{Family: wire.FamReqActivities}, wire.FamActivityRow,
		func(bodies [][]byte) (any, error) { return entities(v, bodies) })

	devs, _ := d.Devices.Decoded.([]*record.Entity)
	acts, _ := d.Activities.Decoded.([]*record.Entity)
	done, total := 0, len(devs)+len(acts)
	for _, ents := range [][]*record.Entity{devs, acts} {
		for _, e := range ents {
			if err := ctx.Err(); err != nil {
				return d, err
			}
			if r.err != nil {
				return d, r.err
			}
			if progress != nil {
				progress(done, total, e.Name)
			}
			done++
			if stale != nil && !stale(e.ID) {
				if kept := old.entity(e.ID); kept != nil {
					cp := *kept
					cp.Name = e.Name
					d.Entities = append(d.Entities, &cp)
					continue
				}
			}
			de := &Entity{ID: e.ID, Name: e.Name}
			id := e.ID
			if record.IsActivityID(id) {
				de.Members = r.reply(fmt.Sprintf("members %#x", id),
					wire.Frame{Family: wire.FamReqActivityMap, Payload: []byte{id}}, wire.FamActivityMapPage,
					func(bodies [][]byte) (any, error) { return entities(v, bodies) })
			} else {
				de.Commands = r.reply(fmt.Sprintf("commands %#x", id),
					wire.Frame{Family: wire.FamReqCommands, Payload: []byte{id, 0xFF}}, wire.FamCommandPage,
					func(bodies [][]byte) (any, error) { return record.ParseCommandList(v, single(bodies)) })
				cmds, _ := de.Commands.Decoded.([]*record.Command)
				kept := old.signals(v, id)
				for _, cmd := range cmds {
					if rep := kept[keyOf(cmd)]; rep != nil {
						de.Data = append(de.Data, rep)
						continue
					}
					de.Data = append(de.Data, r.reply(fmt.Sprintf("data %#x %d", id, cmd.ID),
						wire.Frame{Family: wire.FamReqBlob, Payload: []byte{id, cmd.ID}}, wire.FamBlobPage,
						func(bodies [][]byte) (any, error) {
							_, c, data, err := record.ParseCommandWrite(v, single(bodies))
							if err != nil {
								return nil, err
							}
							return map[string]any{"id": c.ID, "label": c.Label, "data": hex.EncodeToString(data)}, nil
						}))
				}
				de.Inputs = r.reply(fmt.Sprintf("inputs %#x", id),
					wire.Frame{Family: wire.FamReqActivityInputs, Payload: []byte{id}}, wire.FamActivityInputs,
					func(bodies [][]byte) (any, error) { return record.ParseInputs(v, single(bodies)) })
			}
			de.Order = r.reply(fmt.Sprintf("order %#x", id),
				wire.Frame{Family: wire.FamReqOrder, Payload: []byte{id}}, wire.FamOrderReply,
				func(bodies [][]byte) (any, error) {
					_, order, err := record.ParseOrder(single(bodies))
					return order, err
				})
			de.Keymap = r.reply(fmt.Sprintf("keymap %#x", id),
				wire.Frame{Family: wire.FamReqButtons, Payload: []byte{id, 0xFF}}, wire.FamKeymapPage,
				func(bodies [][]byte) (any, error) { return record.ParseKeymap(single(bodies)) })
			de.Macros = r.reply(fmt.Sprintf("macros %#x", id),
				wire.Frame{Family: wire.FamReqMacros, Payload: []byte{id, 0xFF}}, wire.FamMacroPage,
				func(bodies [][]byte) (any, error) {
					var ms []*record.Macro
					for _, b := range bodies {
						m, err := record.ParseMacro(v, b)
						if err != nil {
							return ms, err
						}
						ms = append(ms, m)
					}
					return ms, nil
				})
			d.Entities = append(d.Entities, de)
		}
	}
	if r.err != nil {
		return d, r.err
	}
	if progress != nil {
		progress(total, total, "")
	}
	return d, nil
}

// entity is the entity with the id in d, or nil.
func (d *File) entity(id byte) *Entity {
	for _, e := range d.Entities {
		if e.ID == id {
			return e
		}
	}
	return nil
}

// cmdKey is what identifies a command's signal: a command with the same
// key has the same signal.
type cmdKey struct {
	id, kind byte
	code     [record.CodeLen]byte
	label    string
}

func keyOf(c *record.Command) cmdKey {
	return cmdKey{id: c.ID, kind: byte(c.Type), code: c.Code, label: c.Label}
}

// signals is the device's command data replies in d, by their command's
// key: only replies read without error, of commands d lists.
func (d *File) signals(v wire.Variant, device byte) map[cmdKey]*Reply {
	if d == nil {
		return nil
	}
	for _, e := range d.Entities {
		if e.ID != device {
			continue
		}
		b, ok, err := bodies(e.Commands)
		if err != nil || !ok {
			return nil
		}
		cmds, err := record.ParseCommandList(v, single(b))
		if err != nil {
			return nil
		}
		byID := map[byte]*record.Command{}
		for _, c := range cmds {
			byID[c.ID] = c
		}
		out := map[cmdKey]*Reply{}
		for _, r := range e.Data {
			b, ok, err := bodies(r)
			if err != nil || !ok || r.Error != "" {
				continue
			}
			_, c, _, err := record.ParseCommandWrite(v, single(b))
			if err != nil || byID[c.ID] == nil {
				continue
			}
			out[keyOf(byID[c.ID])] = r
		}
		return out
	}
	return nil
}

// Write writes the dump as indented JSON.
func (d *File) Write(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(d)
}

// Load reads a dump written by Write. Only the raw records matter: the
// decoded views are left as JSON.
func Load(r io.Reader) (*File, error) {
	var d File
	if err := json.NewDecoder(r).Decode(&d); err != nil {
		return nil, fmt.Errorf("dump: %w", err)
	}
	if d.Devices == nil {
		return nil, errors.New("dump: no devices")
	}
	return &d, nil
}

type reader struct {
	ctx context.Context
	c   *hub.Client
	log io.Writer
	// err is the first failure that was not the hub refusing a request:
	// a lost connection, or a hub that stopped answering. The requests
	// after it are not sent.
	err error
}

// reply runs one request and records its outcome. The hub's refusals
// are kept in the dump; other failures end the reading (see reader.err).
func (r *reader) reply(name string, req wire.Frame, fam wire.Family, decode func([][]byte) (any, error)) *Reply {
	rep := &Reply{Request: name}
	if r.err != nil {
		rep.Error = "not read: " + r.err.Error()
		return rep
	}
	bodies, err := r.c.Records(r.ctx, req, fam)
	if err != nil {
		if !errors.Is(err, session.ErrRejected) {
			r.err = fmt.Errorf("dump: %s: %w", name, err)
		}
		rep.Error = err.Error()
		if r.log != nil {
			fmt.Fprintf(r.log, "%s: %v\n", name, err)
		}
		return rep
	}
	for _, b := range bodies {
		rep.Raw = append(rep.Raw, hex.EncodeToString(b))
	}
	if rep.Decoded, err = decode(bodies); err != nil {
		rep.Error = "decoding: " + err.Error()
		if r.log != nil {
			fmt.Fprintf(r.log, "%s: decoding: %v\n", name, err)
		}
	}
	return rep
}

func entities(v wire.Variant, bodies [][]byte) ([]*record.Entity, error) {
	var out []*record.Entity
	for _, b := range bodies {
		e, err := record.ParseEntity(v, b)
		if err != nil {
			return out, err
		}
		out = append(out, e)
	}
	return out, nil
}

func single(bodies [][]byte) []byte {
	if len(bodies) != 1 {
		return nil
	}
	return bodies[0]
}
