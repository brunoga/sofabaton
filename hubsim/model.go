package hubsim

import (
	"slices"

	"github.com/brunoga/sofabaton/record"
	"github.com/brunoga/sofabaton/wire"
)

// Model is a hub configuration the simulated hub serves.
type Model struct {
	Info       *record.HubInfo
	Devices    []*record.Entity
	Activities []*record.Entity
	Commands   map[byte][]*record.Command // by device
	// Data is the commands' signal data, by device and command id.
	Data map[byte]map[byte][]byte
	// Runs records the macros run with family 0x5F.
	Runs    []*record.Macro
	Keymaps map[byte][]record.Binding // by entity
	Macros  map[byte][]*record.Macro  // by entity
	Inputs  map[byte]*record.Inputs   // by device
	Members map[byte][]byte           // activity -> device ids
	// Orders is the devices' command orders (family 0x61/0x62).
	Orders map[byte][]record.Place
	// Idle is the devices' idle behaviors (family 0x41).
	Idle map[byte]byte
	// Synced records the entities synced to the remotes (family 0x65).
	Synced []byte
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
		if len(m.Activities) == 0 {
			return []wire.Frame{Status(NotFound)}
		}
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
		_, c, data, err := record.ParseCommandWrite(v, body)
		if err != nil {
			return []wire.Frame{Status(Rejected)}
		}
		if m.Commands == nil {
			m.Commands = map[byte][]*record.Command{}
		}
		if m.Data == nil {
			m.Data = map[byte]map[byte][]byte{}
		}
		if m.Data[c.Device] == nil {
			m.Data[c.Device] = map[byte][]byte{}
		}
		m.Data[c.Device][c.ID] = data
		cmds := m.Commands[c.Device]
		for i, old := range cmds {
			if old.ID == c.ID { // in place: the hub keeps the sort
				c.Sort = old.Sort
				cmds[i] = c
				return []wire.Frame{Status(0)}
			}
		}
		c.Sort = 0xFF // added commands get no place on the remote
		m.Commands[c.Device] = append(cmds, c)
		return []wire.Frame{Status(0)}
	})
	h.Handle(wire.FamRunMacro, func(req wire.Frame) []wire.Frame {
		_, body, err := wire.SplitPage(req.Payload)
		if err != nil {
			return []wire.Frame{Status(Rejected)}
		}
		v, err := wire.VariantFromModel(m.Info.Model)
		if err != nil {
			return []wire.Frame{Status(Rejected)}
		}
		mac, err := record.ParseMacro(v, body)
		if err != nil {
			return []wire.Frame{Status(Rejected)}
		}
		m.Runs = append(m.Runs, mac)
		return []wire.Frame{Status(0)}
	})
	h.Handle(wire.FamReqBlob, func(req wire.Frame) []wire.Frame {
		if len(req.Payload) != 2 {
			return []wire.Frame{Status(Rejected)}
		}
		dev, id := req.Payload[0], req.Payload[1]
		data := m.Data[dev][id]
		for _, c := range m.Commands[dev] {
			if c.ID == id && data != nil {
				body, err := c.WriteBody(1, data)
				if err != nil {
					return []wire.Frame{Status(Rejected)}
				}
				return Reply(wire.FamBlobPage, body[3:])
			}
		}
		return []wire.Frame{Status(NotFound)}
	})
	h.Handle(wire.FamDeleteKey, func(req wire.Frame) []wire.Frame {
		if len(req.Payload) != 2 {
			return []wire.Frame{Status(Rejected)}
		}
		dev, id := req.Payload[0], req.Payload[1]
		if record.IsActivityID(dev) { // a macro of the activity
			macros := m.Macros[dev]
			for i, mac := range macros {
				if mac.ID == id {
					m.Macros[dev] = append(macros[:i:i], macros[i+1:]...)
					return []wire.Frame{Status(0)}
				}
			}
			return []wire.Frame{Status(NotFound)}
		}
		cmds := m.Commands[dev]
		for i, c := range cmds {
			if c.ID == id {
				m.Commands[dev] = append(cmds[:i:i], cmds[i+1:]...)
				delete(m.Data[dev], id)
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
	// Binding writes replace the key's row; a press that names its command
	// by code alone gets the id of the device's command with that code, as
	// the hub fills it in.
	h.Handle(wire.FamWriteButton, func(req wire.Frame) []wire.Frame {
		p := req.Payload
		if len(p) != 6+record.BindingLen+1 || p[len(p)-1] != wire.Checksum(p[3:len(p)-1]) {
			return []wire.Frame{Status(Rejected)}
		}
		b, _ := record.ParseBinding(p[6 : 6+record.BindingLen])
		for _, press := range []*record.Press{&b.Short, &b.Long} {
			if press.IsZero() || press.Command != 0 {
				continue
			}
			for _, c := range m.Commands[press.Device] {
				if c.Code == press.Code {
					press.Command = c.ID
					break
				}
			}
		}
		if m.Keymaps == nil {
			m.Keymaps = map[byte][]record.Binding{}
		}
		rows := m.Keymaps[b.Entity]
		i := 0
		for i < len(rows) && rows[i].Button != b.Button {
			i++
		}
		if i == len(rows) {
			rows = append(rows, b)
		} else {
			rows[i] = b
		}
		m.Keymaps[b.Entity] = rows
		return []wire.Frame{{Family: wire.FamButtonAck, Payload: []byte{byte(b.Button), byte(b.Button) + 0x3E}}}
	})
	h.Handle(wire.FamReqOrder, func(req wire.Frame) []wire.Frame {
		order, ok := m.Orders[req.Payload[0]]
		if !ok {
			return []wire.Frame{Status(NotFound)}
		}
		content := []byte{req.Payload[0]}
		for _, p := range order {
			content = append(content, p.Command, p.Position)
		}
		return Reply(wire.FamOrderReply, content)
	})
	h.Handle(wire.FamKeySort, func(req wire.Frame) []wire.Frame {
		_, body, err := wire.SplitPage(req.Payload)
		if err != nil || len(body) < 5 || body[len(body)-1] != wire.Checksum(body[:len(body)-1]) {
			return []wire.Frame{Status(Rejected)}
		}
		dev, order, err := record.ParseOrder(body[:len(body)-1])
		if err != nil {
			return []wire.Frame{Status(Rejected)}
		}
		placed := false
		for _, p := range order {
			placed = placed || p.Placed()
		}
		if !placed { // upstream saw status 0x06 for a table placing nothing
			return []wire.Frame{Status(0x06)}
		}
		if m.Orders == nil {
			m.Orders = map[byte][]record.Place{}
		}
		m.Orders[dev] = order
		return []wire.Frame{Status(0)}
	})
	h.Handle(wire.FamCreateDevice, func(req wire.Frame) []wire.Frame {
		e, err := m.entity(req.Payload)
		if err != nil || e.ID != record.NewID {
			return []wire.Frame{Status(Rejected)}
		}
		e.ID = 1
		for _, d := range m.Devices {
			if d.ID >= e.ID {
				e.ID = d.ID + 1
			}
		}
		m.Devices = append(m.Devices, e)
		return []wire.Frame{{Family: wire.FamCreateDeviceAck, Payload: []byte{e.ID}}}
	})
	h.Handle(wire.FamUpdateDevice, func(req wire.Frame) []wire.Frame {
		e, err := m.entity(req.Payload)
		if err != nil {
			return []wire.Frame{Status(Rejected)}
		}
		for i, d := range m.Devices {
			if d.ID == e.ID {
				m.Devices[i] = e
				return []wire.Frame{Status(0)}
			}
		}
		return []wire.Frame{Status(NotFound)}
	})
	h.Handle(wire.FamDeleteEntity, func(req wire.Frame) []wire.Frame {
		id := req.Payload[0]
		for i, d := range m.Devices {
			if d.ID == id {
				m.Devices = append(m.Devices[:i:i], m.Devices[i+1:]...)
				delete(m.Commands, id)
				delete(m.Data, id)
				delete(m.Keymaps, id)
				delete(m.Macros, id)
				delete(m.Orders, id)
				delete(m.Idle, id)
				delete(m.Inputs, id)
				return []wire.Frame{Status(0)}
			}
		}
		return []wire.Frame{Status(NotFound)}
	})
	h.Handle(wire.FamSetIdleBehavior, func(req wire.Frame) []wire.Frame {
		if len(req.Payload) != 2 {
			return []wire.Frame{Status(Rejected)}
		}
		if m.Idle == nil {
			m.Idle = map[byte]byte{}
		}
		m.Idle[req.Payload[0]] = req.Payload[1]
		return []wire.Frame{Status(0)}
	})
	var inputsBody []byte
	h.Handle(wire.FamWriteInputs, func(req wire.Frame) []wire.Frame {
		hdr, chunk, err := wire.SplitPage(req.Payload)
		if err != nil {
			return []wire.Frame{Status(Rejected)}
		}
		if hdr.Page == 1 {
			inputsBody = nil
		}
		inputsBody = append(inputsBody, chunk...)
		if len(inputsBody) < 3 || int(hdr.Page) < int(inputsBody[1])<<8|int(inputsBody[2]) {
			return []wire.Frame{Status(0)}
		}
		v, err := wire.VariantFromModel(m.Info.Model)
		if err != nil {
			return []wire.Frame{Status(Rejected)}
		}
		in, err := record.ParseInputs(v, inputsBody)
		if err != nil {
			return []wire.Frame{Status(Rejected)}
		}
		if m.Inputs == nil {
			m.Inputs = map[byte]*record.Inputs{}
		}
		m.Inputs[in.Device] = in
		return []wire.Frame{Status(0)}
	})
	h.Handle(wire.FamAssignCommit, func(req wire.Frame) []wire.Frame {
		m.Synced = append(m.Synced, req.Payload[0])
		return []wire.Frame{Status(0)}
	})
	var macroBody []byte
	h.Handle(wire.FamWriteMacro, func(req wire.Frame) []wire.Frame {
		hdr, chunk, err := wire.SplitPage(req.Payload)
		if err != nil {
			return []wire.Frame{Status(Rejected)}
		}
		if hdr.Page == 1 {
			macroBody = nil
		}
		macroBody = append(macroBody, chunk...)
		if len(macroBody) < 3 || int(hdr.Page) < int(macroBody[1])<<8|int(macroBody[2]) {
			return []wire.Frame{Status(0)}
		}
		v, err := wire.VariantFromModel(m.Info.Model)
		if err != nil {
			return []wire.Frame{Status(Rejected)}
		}
		mac, err := record.ParseMacro(v, macroBody)
		if err != nil {
			return []wire.Frame{Status(Rejected)}
		}
		if m.Macros == nil {
			m.Macros = map[byte][]*record.Macro{}
		}
		macros := m.Macros[mac.Entity]
		if mac.ID == 0 { // a new macro: the hub gives it a free id
			used := map[byte]bool{}
			for _, o := range macros {
				used[o.ID] = true
			}
			mac.ID = 1
			for used[mac.ID] {
				mac.ID++
			}
		}
		if record.IsActivityID(mac.Entity) && mac.ID == record.MacroPowerOn {
			// The hub takes an activity's members from its POWER_ON
			// macro, in order.
			var members []byte
			for _, st := range mac.Steps {
				if st.Command == record.MacroPowerOn && !slices.Contains(members, st.Device) {
					members = append(members, st.Device)
				}
			}
			if m.Members == nil {
				m.Members = map[byte][]byte{}
			}
			m.Members[mac.Entity] = members
		}
		i := 0
		for i < len(macros) && macros[i].ID != mac.ID {
			i++
		}
		if i == len(macros) {
			macros = append(macros, mac)
		} else {
			macros[i] = mac
		}
		m.Macros[mac.Entity] = macros
		return []wire.Frame{{Family: wire.FamMacroAck, Payload: []byte{mac.ID}}}
	})
	var confirmed []byte
	h.Handle(wire.FamConfirmMember, func(req wire.Frame) []wire.Frame {
		if len(req.Payload) != 2 || req.Payload[1] != 0 {
			return []wire.Frame{Status(Rejected)}
		}
		confirmed = append(confirmed, req.Payload[0])
		return []wire.Frame{Status(0)}
	})
	h.Handle(wire.FamCreateActivity, func(req wire.Frame) []wire.Frame {
		e, err := m.entity(req.Payload)
		if err != nil || e.ID != record.NewID {
			return []wire.Frame{Status(Rejected)}
		}
		e.ID = record.MinActivityID
		for _, a := range m.Activities {
			if a.ID >= e.ID {
				e.ID = a.ID + 1
			}
		}
		e.Sort = byte(len(m.Activities) + 1)
		m.Activities = append(m.Activities, e)
		if m.Members == nil {
			m.Members = map[byte][]byte{}
		}
		m.Members[e.ID], confirmed = confirmed, nil
		return []wire.Frame{{Family: wire.FamCreateActAck, Payload: []byte{e.ID}}}
	})
	h.Handle(wire.FamWriteActivity, func(req wire.Frame) []wire.Frame {
		e, err := m.entity(req.Payload)
		if err != nil {
			return []wire.Frame{Status(Rejected)}
		}
		for i, a := range m.Activities {
			if a.ID == e.ID {
				m.Activities[i] = e
				return []wire.Frame{Status(0)}
			}
		}
		return []wire.Frame{Status(NotFound)}
	})
	h.Handle(wire.FamDeleteActivity, func(req wire.Frame) []wire.Frame {
		id := req.Payload[0]
		for i, a := range m.Activities {
			if a.ID == id {
				m.Activities = append(m.Activities[:i:i], m.Activities[i+1:]...)
				delete(m.Keymaps, id)
				delete(m.Macros, id)
				delete(m.Members, id)
				delete(m.Orders, id)
				return []wire.Frame{Status(0)}
			}
		}
		return []wire.Frame{Status(NotFound)}
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

// entity decodes the record of a create or update write.
func (m *Model) entity(payload []byte) (*record.Entity, error) {
	v, err := wire.VariantFromModel(m.Info.Model)
	if err != nil {
		return nil, err
	}
	_, body, err := wire.SplitPage(payload)
	if err != nil {
		return nil, err
	}
	return record.ParseEntity(v, body)
}

func entityRows(fam wire.Family, entities []*record.Entity) []wire.Frame {
	var recs [][]byte
	for _, e := range entities {
		body, _ := e.MarshalBinary()
		recs = append(recs, body[3:])
	}
	return Reply(fam, recs...)
}
