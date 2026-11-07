package hubsim

import (
	"crypto/md5" //nolint:gosec // G501: the hub checks firmware images by MD5
	"encoding/binary"
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
	// OrderSyncs counts the order syncs (family 0x64).
	OrderSyncs int
	// Paired records the Bluetooth devices paired (family 0x4C).
	Paired []byte
	// Remotes are the RemoteStatus payloads the hub answers ReqRemoteList
	// with, one frame each.
	Remotes [][]byte
	// Learned is the command data the hub learns when asked to (family
	// 0x04), after LearnFailures failed attempts; nil, it learns nothing
	// and closes the window (family 0x05).
	Learned       []byte
	LearnFailures int
	// LearnArms counts the times learning was armed, LearnLeft whether
	// it was left (family 0x05).
	LearnArms int
	LearnLeft bool
	// Firmware is the last firmware image the hub took whole (families
	// 0x29 and 0x2A), its size and MD5 as the start gave them.
	Firmware []byte

	// unpaired is the Bluetooth devices created and not paired yet,
	// which the hub does not list.
	unpaired map[byte]bool
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
		var listed []*record.Entity
		for _, d := range m.Devices {
			if !m.unpaired[d.ID] {
				listed = append(listed, d)
			}
		}
		if len(listed) == 0 {
			return []wire.Frame{Status(NotFound)}
		}
		return entityRows(wire.FamDeviceRow, listed)
	})
	h.Handle(wire.FamReqActivities, func(wire.Frame) []wire.Frame {
		if len(m.Activities) == 0 {
			return []wire.Frame{Status(NotFound)}
		}
		return entityRows(wire.FamActivityRow, m.Activities)
	})
	h.Handle(wire.FamReqActivityMap, func(req wire.Frame) []wire.Frame {
		ids, ok := m.Members[req.Payload[0]]
		if !ok {
			return []wire.Frame{Status(NotFound)}
		}
		var members []*record.Entity
		for _, id := range ids {
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
		if record.IsActivityID(dev) { // a macro, favorite or key of the activity
			macros := m.Macros[dev]
			for i, mac := range macros {
				if mac.ID == id {
					m.Macros[dev] = append(macros[:i:i], macros[i+1:]...)
					return []wire.Frame{Status(0)}
				}
			}
			rows := m.Keymaps[dev]
			for i, b := range rows {
				if byte(b.Button) == id { // a favorite or a key
					m.Keymaps[dev] = append(rows[:i:i], rows[i+1:]...)
					return []wire.Frame{Status(0)}
				}
			}
			return []wire.Frame{Status(NotFound)}
		}
		if record.Button(id).IsHardKey() { // a key of the device
			rows := m.Keymaps[dev]
			for i, b := range rows {
				if byte(b.Button) == id {
					m.Keymaps[dev] = append(rows[:i:i], rows[i+1:]...)
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
		if record.IsActivityID(b.Entity) && b.Button == 0 { // a new favorite
			if b.Short.Command > 199 {
				return []wire.Frame{Status(0x09)}
			}
			b.Button = record.Button(m.freeID(b.Entity))
			m.Keymaps[b.Entity] = append(m.Keymaps[b.Entity], b)
			return []wire.Frame{{Family: wire.FamButtonAck, Payload: []byte{byte(b.Button), byte(b.Button) + 0x3E}}}
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
		e.ID = freeEntityID(m.Devices, 1)
		e.Sort = byte(len(m.Devices) + 1) // our X2 puts a new device last
		m.Devices = append(m.Devices, e)
		if e.Class == record.ClassBluetooth {
			// Our X2 lists a new Bluetooth device once it has paired.
			if m.unpaired == nil {
				m.unpaired = map[byte]bool{}
			}
			m.unpaired[e.ID] = true
		}
		return []wire.Frame{{Family: wire.FamCreateDeviceAck, Payload: []byte{e.ID}}}
	})
	h.Handle(wire.FamUpdateDevice, func(req wire.Frame) []wire.Frame {
		e, err := m.entity(req.Payload)
		if err != nil {
			return []wire.Frame{Status(Rejected)}
		}
		for i, d := range m.Devices {
			if d.ID == e.ID {
				e.Sort = d.Sort // our X2 keeps its own
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
	sortHandler := func(ents *[]*record.Entity) Handler {
		return func(req wire.Frame) []wire.Frame {
			_, body, err := wire.SplitPage(req.Payload)
			if err != nil || len(body) < 4 || body[len(body)-1] != wire.Checksum(body[:len(body)-1]) {
				return []wire.Frame{Status(Rejected)}
			}
			ids, err := record.ParseEntitySort(body)
			if err != nil || len(ids) != len(*ents) {
				return []wire.Frame{Status(Rejected)}
			}
			place := map[byte]byte{}
			for i, id := range ids {
				place[id] = byte(i + 1)
			}
			for _, e := range *ents {
				if place[e.ID] == 0 {
					return []wire.Frame{Status(Rejected)}
				}
			}
			for _, e := range *ents {
				e.Sort = place[e.ID]
			}
			return []wire.Frame{Status(0)}
		}
	}
	h.Handle(wire.FamDeviceSort, sortHandler(&m.Devices))
	h.Handle(wire.FamActivitySort, sortHandler(&m.Activities))
	h.Handle(wire.FamPair, func(req wire.Frame) []wire.Frame {
		if len(req.Payload) != 2 || m.device(req.Payload[0]) == nil || m.device(req.Payload[0]).Class != record.ClassBluetooth {
			return []wire.Frame{Status(Rejected)}
		}
		m.Paired = append(m.Paired, req.Payload[0])
		delete(m.unpaired, req.Payload[0])
		// The device pairs at once.
		return []wire.Frame{Status(0), {Family: wire.FamPaired, Payload: req.Payload[:1]}}
	})
	h.Handle(wire.FamLearnExit, func(wire.Frame) []wire.Frame {
		m.LearnLeft = true
		return nil
	})
	h.Handle(wire.FamLearnEnter, func(wire.Frame) []wire.Frame {
		m.LearnArms++
		out := []wire.Frame{Status(0)}
		for range m.LearnFailures {
			out = append(out, wire.Frame{Family: wire.FamLearnFailed})
		}
		if m.Learned == nil {
			return append(out, wire.Frame{Family: wire.FamLearnExpired})
		}
		// The hub sends the code as the body of a command write of
		// device 0, command 0, type 0 and no label.
		v, err := wire.VariantFromModel(m.Info.Model)
		if err != nil {
			return []wire.Frame{Status(Rejected)}
		}
		cmd, err := record.NewCommand(v)
		if err != nil {
			return []wire.Frame{Status(Rejected)}
		}
		pages, err := cmd.WritePages(1, 1, m.Learned)
		if err != nil {
			return []wire.Frame{Status(Rejected)}
		}
		for _, p := range pages {
			out = append(out, wire.Frame{Family: wire.FamLearnData, Payload: p})
		}
		return out
	})
	h.Handle(wire.FamReqRemoteList, func(wire.Frame) []wire.Frame {
		var out []wire.Frame
		for _, r := range m.Remotes {
			out = append(out, wire.Frame{Family: wire.FamRemoteStatus, Payload: r})
		}
		return out
	})
	var fwSize int
	var fwSum, fwImage []byte
	fwAck := func(n int) []wire.Frame {
		return []wire.Frame{{Family: wire.FamFirmwareAck, Payload: binary.BigEndian.AppendUint16([]byte{0, 0, 1, 0, 0, 0}, uint16(n))}}
	}
	h.Handle(wire.FamFirmwareStart, func(req wire.Frame) []wire.Frame {
		if len(req.Payload) != 26 {
			return []wire.Frame{Status(Rejected)}
		}
		fwSize, fwSum, fwImage = int(binary.BigEndian.Uint32(req.Payload[6:])), req.Payload[10:], nil
		return fwAck(0)
	})
	h.Handle(wire.FamFirmwareChunk, func(req wire.Frame) []wire.Frame {
		if fwSum == nil || len(req.Payload) < 10 || int(binary.BigEndian.Uint16(req.Payload[7:])) != len(fwImage)/record.FirmwareChunkLen+1 {
			return []wire.Frame{Status(Rejected)}
		}
		fwImage = append(fwImage, req.Payload[9:]...)
		if len(fwImage) < fwSize {
			return fwAck(int(binary.BigEndian.Uint16(req.Payload[7:])))
		}
		if sum := md5.Sum(fwImage); len(fwImage) != fwSize || string(sum[:]) != string(fwSum) { //nolint:gosec // G401: the hub's check, not a security one
			return []wire.Frame{Status(Rejected)}
		}
		m.Firmware, fwSum = fwImage, nil
		return []wire.Frame{{Family: wire.FamFirmwareDone, Payload: []byte{0, 0, 1, 0, 0, 0}}}
	})
	h.Handle(wire.FamRemoteSync, func(wire.Frame) []wire.Frame {
		m.OrderSyncs++
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
			mac.ID = m.freeID(mac.Entity)
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
		e.ID = freeEntityID(m.Activities, record.MinActivityID)
		e.Sort = byte(len(m.Activities) + 1)
		m.Activities = append(m.Activities, e)
		// The members confirmed are not its members yet: the hub has
		// none for a new activity (its map is not found) until its
		// POWER_ON macro is written (our X2, 2026-11-04).
		confirmed = nil
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

// freeEntityID is the lowest id from first that no entity has: the id the
// hub gives a new device or activity (our X2 gave a new device id 1, freed
// by a delete, with devices up to 5 in use).
func freeEntityID(ents []*record.Entity, first byte) byte {
	used := map[byte]bool{}
	for _, e := range ents {
		used[e.ID] = true
	}
	id := first
	for used[id] {
		id++
	}
	return id
}

// freeID is the lowest id an entity's new macro or favorite gets: an
// activity's macros and favorites share one id space.
func (m *Model) freeID(entity byte) byte {
	used := map[byte]bool{}
	for _, o := range m.Macros[entity] {
		used[o.ID] = true
	}
	for _, b := range m.Keymaps[entity] {
		if !b.Button.IsHardKey() {
			used[byte(b.Button)] = true
		}
	}
	id := byte(1)
	for used[id] {
		id++
	}
	return id
}

// device is the device with the id, or nil.
func (m *Model) device(id byte) *record.Entity {
	for _, d := range m.Devices {
		if d.ID == id {
			return d
		}
	}
	return nil
}
