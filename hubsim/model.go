package hubsim

import (
	"github.com/brunoga/sofabaton/record"
	"github.com/brunoga/sofabaton/wire"
)

// Model is a hub configuration the simulated hub serves.
type Model struct {
	Info       *record.HubInfo
	Devices    []*record.Entity
	Activities []*record.Entity
	Commands   map[byte][]*record.Command // by device
	Keymaps    map[byte][]record.Binding  // by entity
	Macros     map[byte][]*record.Macro   // by entity
	Inputs     map[byte]*record.Inputs    // by device
	Members    map[byte][]byte            // activity -> device ids
}

// NotFound is the status the simulated hub rejects requests for absent
// data with.
const NotFound = 0x07

// Rejected is the status the simulated hub rejects malformed writes with
// (the status upstream saw for a bad save).
const Rejected = 0x0C

// FromModel returns a hub that answers the readers' requests from m.
func FromModel(m *Model) *Hub {
	h := New()
	h.Handle(wire.FamReqBanner, func(wire.Frame) []wire.Frame {
		return []wire.Frame{{Family: wire.FamBanner, Payload: m.Info.AppendPayload(nil)}}
	})
	h.Handle(wire.FamReqDevices, func(wire.Frame) []wire.Frame {
		return entityRows(wire.FamDeviceRow, m.Devices)
	})
	h.Handle(wire.FamReqActivities, func(wire.Frame) []wire.Frame {
		return entityRows(wire.FamActivityRow, m.Activities)
	})
	h.Handle(wire.FamReqActivityMap, func(req wire.Frame) []wire.Frame {
		var members []*record.Entity
		for _, id := range m.Members[req.Payload[0]] {
			for _, d := range m.Devices {
				if d.ID == id {
					members = append(members, d)
				}
			}
		}
		return entityRows(wire.FamActivityMapPage, members)
	})
	h.Handle(wire.FamReqCommands, func(req wire.Frame) []wire.Frame {
		cmds, ok := m.Commands[req.Payload[0]]
		if !ok {
			return []wire.Frame{Status(NotFound)}
		}
		content := []byte{byte(len(cmds))}
		for _, c := range cmds {
			content, _ = c.AppendRecord(content)
		}
		return Reply(wire.FamCommandPage, content)
	})
	// Command writes (family 0x0E) add to the device's list, the way the
	// hub stores a single added command; the hub's own id and sort
	// assignment in device creates is not modelled.
	writes := map[byte][]byte{} // burst seq -> body so far
	h.Handle(wire.FamWriteCommand, func(req wire.Frame) []wire.Frame {
		hdr, chunk, err := wire.SplitPage(req.Payload)
		if err != nil {
			return []wire.Frame{Status(Rejected)}
		}
		body := append(writes[hdr.Marker], chunk...)
		if len(body) < 3 || int(hdr.Page) < int(body[1])<<8|int(body[2]) {
			writes[hdr.Marker] = body
			return []wire.Frame{Status(0)}
		}
		delete(writes, hdr.Marker)
		v, err := wire.VariantFromModel(m.Info.Model)
		if err != nil {
			return []wire.Frame{Status(Rejected)}
		}
		_, c, _, err := record.ParseCommandWrite(v, body)
		if err != nil {
			return []wire.Frame{Status(Rejected)}
		}
		c.Sort = c.ID
		if m.Commands == nil {
			m.Commands = map[byte][]*record.Command{}
		}
		m.Commands[c.Device] = append(m.Commands[c.Device], c)
		return []wire.Frame{Status(0)}
	})
	h.Handle(wire.FamDeleteKey, func(req wire.Frame) []wire.Frame {
		if len(req.Payload) != 2 {
			return []wire.Frame{Status(Rejected)}
		}
		dev, id := req.Payload[0], req.Payload[1]
		cmds := m.Commands[dev]
		for i, c := range cmds {
			if c.ID == id {
				m.Commands[dev] = append(cmds[:i:i], cmds[i+1:]...)
				return []wire.Frame{Status(0)}
			}
		}
		return []wire.Frame{Status(NotFound)}
	})
	h.Handle(wire.FamReqButtons, func(req wire.Frame) []wire.Frame {
		rows, ok := m.Keymaps[req.Payload[0]]
		if !ok {
			return []wire.Frame{Status(NotFound)}
		}
		content := []byte{byte(len(rows))}
		for _, r := range rows {
			content, _ = r.AppendBinary(content)
		}
		return Reply(wire.FamKeymapPage, content)
	})
	h.Handle(wire.FamReqMacros, func(req wire.Frame) []wire.Frame {
		macros, ok := m.Macros[req.Payload[0]]
		if !ok {
			return []wire.Frame{Status(NotFound)}
		}
		var recs [][]byte
		for _, mac := range macros {
			body, _ := mac.Body()
			recs = append(recs, body[3:])
		}
		return Reply(wire.FamMacroPage, recs...)
	})
	h.Handle(wire.FamReqActivityInputs, func(req wire.Frame) []wire.Frame {
		in, ok := m.Inputs[req.Payload[0]]
		if !ok {
			return []wire.Frame{Status(NotFound)}
		}
		body, _ := in.Body()
		return Reply(wire.FamActivityInputs, body[3:])
	})
	return h
}

func entityRows(fam wire.Family, entities []*record.Entity) []wire.Frame {
	var recs [][]byte
	for _, e := range entities {
		body, _ := e.MarshalBinary()
		recs = append(recs, body[3:])
	}
	return Reply(fam, recs...)
}
