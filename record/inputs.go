package record

import (
	"fmt"

	"github.com/brunoga/sofabaton/wire"
)

// Inputs page layout.
const (
	inputsHeaderLen  = 8
	controlKeyLen    = 9
	controlKeyCount  = 4
	favoriteLen      = 7
	favoriteCount    = 10
	inputsTrailerLen = controlKeyCount*controlKeyLen + favoriteCount*favoriteLen + 1
)

// Input modes of a device's inputs page.
const (
	InputsNone     byte = 0x00 // no inputs configured
	InputsDirect   byte = 0x01 // each input is its own command
	InputsNoSwitch byte = 0x02 // no input switching
)

// Control keys of an inputs page, in wire order.
const (
	InputKeyList = iota
	InputKeyUp
	InputKeyDown
	InputKeyConfirm
)

// InputEntry is one of a device's inputs: the command that selects it.
type InputEntry struct {
	Command byte
	Code    [CodeLen]byte
	// Ordinal is the 1-based position the X1S/X2 store; X1 entries have
	// none (0).
	Ordinal byte
	Label   string

	slot []byte
}

// Inputs is a device's inputs page: the input list, the four input
// control keys and ten favorite rows. The hub sends it (family 0x47, in
// reply to a request with the device id) with the layout the app writes it
// with (family 0x46).
type Inputs struct {
	Device byte
	Mode   byte
	// FlagA and FlagB are the start and restart positions (0 observed).
	FlagA, FlagB byte
	Entries      []InputEntry
	// ControlKeys and Favorites are opaque rows, kept as they are.
	ControlKeys [controlKeyCount][controlKeyLen]byte
	Favorites   [favoriteCount][favoriteLen]byte
	State       byte

	variant wire.Variant
	slack   []byte // bytes between the entries and the trailer, if any
}

// NewInputs returns an empty inputs page for a device of variant v.
func NewInputs(v wire.Variant, device, mode byte) (*Inputs, error) {
	if _, err := v.Schema(); err != nil {
		return nil, err
	}
	return &Inputs{Device: device, Mode: mode, variant: v}, nil
}

// ParseInputs decodes an assembled inputs body of variant v.
func ParseInputs(v wire.Variant, body []byte) (*Inputs, error) {
	s, err := v.Schema()
	if err != nil {
		return nil, err
	}
	if len(body) < inputsHeaderLen+inputsTrailerLen+1 {
		return nil, fmt.Errorf("record: inputs page of %d bytes", len(body))
	}
	n := int(body[5])
	entriesEnd := inputsHeaderLen + n*s.InputEntryStride
	trailer := len(body) - 1 - inputsTrailerLen
	if entriesEnd > trailer {
		return nil, fmt.Errorf("record: inputs page with %d entries is %d bytes", n, len(body))
	}
	in := &Inputs{
		Device: body[3], Mode: body[4], FlagA: body[6], FlagB: body[7],
		Entries: make([]InputEntry, 0, n),
		State:   body[len(body)-2],
		variant: v,
	}
	if entriesEnd < trailer {
		in.slack = append([]byte(nil), body[entriesEnd:trailer]...)
	}
	for i := range n {
		row := body[inputsHeaderLen+i*s.InputEntryStride : inputsHeaderLen+(i+1)*s.InputEntryStride]
		e := InputEntry{Command: row[0]}
		copy(e.Code[:], row[1:7])
		label := row[7:]
		if s.Labels == wire.UTF16BE {
			e.Ordinal = row[7]
			label = row[8:]
		}
		e.slot = append([]byte(nil), label...)
		e.Label = s.Labels.DecodeSlot(label)
		in.Entries = append(in.Entries, e)
	}
	t := body[trailer : len(body)-1]
	for i := range in.ControlKeys {
		copy(in.ControlKeys[i][:], t[i*controlKeyLen:])
	}
	t = t[controlKeyCount*controlKeyLen:]
	for i := range in.Favorites {
		copy(in.Favorites[i][:], t[i*favoriteLen:])
	}
	return in, nil
}

// Variant is the hub variant the page belongs to.
func (in *Inputs) Variant() wire.Variant { return in.variant }

// Body returns the page's write body, checksummed. Entries whose label is
// unchanged keep their slot's bytes.
func (in *Inputs) Body() ([]byte, error) {
	s, err := in.variant.Schema()
	if err != nil {
		return nil, err
	}
	if len(in.Entries) > 0xFF {
		return nil, fmt.Errorf("record: %d inputs, at most 255 fit", len(in.Entries))
	}
	n := inputsHeaderLen + len(in.Entries)*s.InputEntryStride + len(in.slack) + inputsTrailerLen + 1
	pages := wire.PageCount(n)
	b := make([]byte, 0, n)
	b = append(b, 0x01, byte(pages>>8), byte(pages), in.Device, in.Mode, byte(len(in.Entries)), in.FlagA, in.FlagB)
	for _, e := range in.Entries {
		b = append(b, e.Command)
		b = append(b, e.Code[:]...)
		if s.Labels == wire.UTF16BE {
			b = append(b, e.Ordinal)
		}
		if e.slot != nil && len(e.slot) == s.InputLabelLen && s.Labels.DecodeSlot(e.slot) == e.Label {
			b = append(b, e.slot...)
		} else {
			b = s.Labels.AppendSlot(b, e.Label, s.InputLabelLen)
		}
	}
	b = append(b, in.slack...)
	for _, k := range in.ControlKeys {
		b = append(b, k[:]...)
	}
	for _, f := range in.Favorites {
		b = append(b, f[:]...)
	}
	b = append(b, in.State)
	return append(b, wire.Checksum(b)), nil
}

// WritePages returns the payloads of the page's write (family 0x46), one
// per page, each acked by a status ack.
func (in *Inputs) WritePages() ([][]byte, error) {
	body, err := in.Body()
	if err != nil {
		return nil, err
	}
	return wire.Pages(0x01, body), nil
}
