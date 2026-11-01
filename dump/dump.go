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

// Read dumps the hub c is connected to. A request that fails is kept in
// the dump, with its error, and reported to log.
func Read(ctx context.Context, c *hub.Client, tool string, log io.Writer) *File {
	i := c.Info()
	d := &File{
		Tool: tool,
		Info: Info{Name: i.Name, Model: i.Variant.String(), Firmware: int(i.Firmware),
			Batch: i.BatchDate(), ID: hex.EncodeToString(i.ID[:]), Raw: hex.EncodeToString(i.AppendPayload(nil))},
	}
	v := c.Variant()
	r := reader{ctx: ctx, c: c, log: log}
	d.Devices = r.reply("devices", wire.Frame{Family: wire.FamReqDevices}, wire.FamDeviceRow,
		func(bodies [][]byte) (any, error) { return entities(v, bodies) })
	d.Activities = r.reply("activities", wire.Frame{Family: wire.FamReqActivities}, wire.FamActivityRow,
		func(bodies [][]byte) (any, error) { return entities(v, bodies) })

	for _, list := range []*Reply{d.Devices, d.Activities} {
		ents, _ := list.Decoded.([]*record.Entity)
		for _, e := range ents {
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
				for _, cmd := range cmds {
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
	return d
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
}

// reply runs one request and records its outcome; failures are kept in
// the dump rather than ending it.
func (r reader) reply(name string, req wire.Frame, fam wire.Family, decode func([][]byte) (any, error)) *Reply {
	rep := &Reply{Request: name}
	bodies, err := r.c.Records(r.ctx, req, fam)
	if err != nil {
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
