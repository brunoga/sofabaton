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
