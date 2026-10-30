package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/brunoga/sofabaton/hub"
	"github.com/brunoga/sofabaton/record"
	"github.com/brunoga/sofabaton/wire"
)

// The dump keeps every reply's raw record bodies (hex) next to a decoded
// view, so it is a lossless backup even of bytes no decoder understands.

type dumpFile struct {
	Tool       string        `json:"tool"`
	Info       dumpInfo      `json:"info"`
	Devices    *dumpReply    `json:"devices"`
	Activities *dumpReply    `json:"activities"`
	Entities   []*dumpEntity `json:"entities"`
}

type dumpInfo struct {
	Name     string `json:"name"`
	Model    string `json:"model"`
	Firmware int    `json:"firmware"`
	Batch    string `json:"batch"`
	ID       string `json:"id"`
	Raw      string `json:"raw"`
}

// dumpReply is one request's reply: raw records and the decoded view, or
// the error the request failed with.
type dumpReply struct {
	Request string   `json:"request"`
	Raw     []string `json:"raw,omitempty"`
	Decoded any      `json:"decoded,omitempty"`
	Error   string   `json:"error,omitempty"`
}

type dumpEntity struct {
	ID       byte       `json:"id"`
	Name     string     `json:"name"`
	Commands *dumpReply `json:"commands,omitempty"`
	Keymap   *dumpReply `json:"keymap,omitempty"`
	Macros   *dumpReply `json:"macros,omitempty"`
	Inputs   *dumpReply `json:"inputs,omitempty"`
	Members  *dumpReply `json:"members,omitempty"`
}

func dump(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("dump", flag.ContinueOnError)
	var h hubFlags
	h.register(fs)
	out := fs.String("o", "", "write the dump to this file instead of stdout")
	if err := fs.Parse(args); err != nil {
		return err
	}
	c, err := h.connect(ctx, stderr)
	if err != nil {
		return err
	}
	defer func() { _ = c.Close() }()

	i := c.Info()
	d := dumpFile{
		Tool: "sbx " + version,
		Info: dumpInfo{Name: i.Name, Model: i.Variant.String(), Firmware: int(i.Firmware),
			Batch: i.BatchDate(), ID: hex.EncodeToString(i.ID[:]), Raw: hex.EncodeToString(i.AppendPayload(nil))},
	}
	v := c.Variant()
	devices := reply(ctx, c, stderr, "devices", wire.Frame{Family: wire.FamReqDevices}, wire.FamDeviceRow,
		func(bodies [][]byte) (any, error) { return entities(v, bodies) })
	activities := reply(ctx, c, stderr, "activities", wire.Frame{Family: wire.FamReqActivities}, wire.FamActivityRow,
		func(bodies [][]byte) (any, error) { return entities(v, bodies) })
	d.Devices, d.Activities = devices, activities

	for _, list := range []*dumpReply{devices, activities} {
		ents, _ := list.Decoded.([]*record.Entity)
		for _, e := range ents {
			de := &dumpEntity{ID: e.ID, Name: e.Name}
			id := e.ID
			if record.IsActivityID(id) {
				de.Members = reply(ctx, c, stderr, fmt.Sprintf("members %#x", id),
					wire.Frame{Family: wire.FamReqActivityMap, Payload: []byte{id}}, wire.FamActivityMapPage,
					func(bodies [][]byte) (any, error) { return entities(v, bodies) })
			} else {
				de.Commands = reply(ctx, c, stderr, fmt.Sprintf("commands %#x", id),
					wire.Frame{Family: wire.FamReqCommands, Payload: []byte{id, 0xFF}}, wire.FamCommandPage,
					func(bodies [][]byte) (any, error) { return record.ParseCommandList(v, single(bodies)) })
				de.Inputs = reply(ctx, c, stderr, fmt.Sprintf("inputs %#x", id),
					wire.Frame{Family: wire.FamReqActivityInputs, Payload: []byte{id}}, wire.FamActivityInputs,
					func(bodies [][]byte) (any, error) { return record.ParseInputs(v, single(bodies)) })
			}
			de.Keymap = reply(ctx, c, stderr, fmt.Sprintf("keymap %#x", id),
				wire.Frame{Family: wire.FamReqButtons, Payload: []byte{id, 0xFF}}, wire.FamKeymapPage,
				func(bodies [][]byte) (any, error) { return record.ParseKeymap(single(bodies)) })
			de.Macros = reply(ctx, c, stderr, fmt.Sprintf("macros %#x", id),
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

	w := stdout
	if *out != "" {
		f, err := os.Create(*out)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		w = f
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(d)
}

// reply runs one request and records its outcome; failures are kept in
// the dump rather than ending it.
func reply(ctx context.Context, c *hub.Client, stderr io.Writer, name string, req wire.Frame, fam wire.Family,
	decode func([][]byte) (any, error)) *dumpReply {
	r := &dumpReply{Request: name}
	bodies, err := c.Records(ctx, req, fam)
	if err != nil {
		r.Error = err.Error()
		fmt.Fprintf(stderr, "%s: %v\n", name, err)
		return r
	}
	for _, b := range bodies {
		r.Raw = append(r.Raw, hex.EncodeToString(b))
	}
	if r.Decoded, err = decode(bodies); err != nil {
		r.Error = "decoding: " + err.Error()
		fmt.Fprintf(stderr, "%s: decoding: %v\n", name, err)
	}
	return r
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
