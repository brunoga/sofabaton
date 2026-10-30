package record

import (
	"fmt"
	"unicode/utf8"

	"github.com/brunoga/sofabaton/wire"
)

// HubIDLen is the length of the hub id that leads the banner and the
// discovery reply.
const HubIDLen = 6

const hubInfoFixedLen = HubIDLen + 9

// HubInfo is a hub's identity, from the banner (family 0x02, the reply to
// a banner request) or the discovery reply (family 0xC2). Both carry the
// same payload.
type HubInfo struct {
	// ID identifies the hub: its MAC on the X2; the first five MAC bytes
	// and a model suffix (0x4B X1, 0x45 X1S) on the others. Discovery
	// matches CALL_ME frames to hubs by it.
	ID [HubIDLen]byte
	// Marker is 0x64 in discovery replies and 0x00 or 0x64 in banners.
	Marker  byte
	Variant wire.Variant
	// Model is the raw model code, kept when Variant is unknown.
	Model byte
	// Batch is the production batch, a date packed as BCD (2022-11-20).
	Batch [4]byte
	// Firmware is the hub firmware version (the mDNS AVER value).
	Firmware byte
	// Flags differ between models and revisions; do not rely on them.
	Flags [2]byte
	// Name is the hub's name. The app writes GB2312 above ASCII; a name
	// that is not UTF-8 is read as Latin-1, and RawName keeps its bytes.
	Name    string
	RawName []byte
}

// ParseHubInfo decodes a banner or discovery-reply payload. An unknown
// model is not an error: Variant is left zero and Model holds the code.
func ParseHubInfo(payload []byte) (*HubInfo, error) {
	if len(payload) < hubInfoFixedLen {
		return nil, fmt.Errorf("record: hub info of %d bytes", len(payload))
	}
	h := &HubInfo{
		Marker:   payload[6],
		Model:    payload[7],
		Firmware: payload[12],
		RawName:  append([]byte(nil), payload[hubInfoFixedLen:]...),
	}
	copy(h.ID[:], payload[:HubIDLen])
	copy(h.Batch[:], payload[8:12])
	copy(h.Flags[:], payload[13:15])
	if v, err := wire.VariantFromModel(h.Model); err == nil {
		h.Variant = v
	}
	h.Name = hubName(h.RawName)
	return h, nil
}

func hubName(b []byte) string {
	if utf8.Valid(b) {
		return string(b)
	}
	r := make([]rune, len(b))
	for i, c := range b {
		r[i] = rune(c)
	}
	return string(r)
}

// BatchDate returns the production batch as YYYY-MM-DD, or "" when the
// bytes are not BCD.
func (h *HubInfo) BatchDate() string {
	var digits [8]byte
	for i, b := range h.Batch {
		hi, lo := b>>4, b&0x0F
		if hi > 9 || lo > 9 {
			return ""
		}
		digits[2*i], digits[2*i+1] = '0'+hi, '0'+lo
	}
	return fmt.Sprintf("%s-%s-%s", digits[0:4], digits[4:6], digits[6:8])
}

// AppendPayload appends the banner/discovery payload to b.
func (h *HubInfo) AppendPayload(b []byte) []byte {
	b = append(b, h.ID[:]...)
	b = append(b, h.Marker, h.Model)
	b = append(b, h.Batch[:]...)
	b = append(b, h.Firmware)
	b = append(b, h.Flags[:]...)
	if h.RawName != nil {
		return append(b, h.RawName...)
	}
	return append(b, h.Name...)
}
