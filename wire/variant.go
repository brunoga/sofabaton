package wire

import (
	"errors"
	"fmt"
)

// Variant is a hub model line. The models share the protocol but differ in
// record dimensions and text encoding.
type Variant uint8

const (
	X1 Variant = iota + 1
	X1S
	X2
)

// ErrUnknownVariant reports a model this package has no wire layout for.
// Guessing one would corrupt writes, so callers must not fall back.
var ErrUnknownVariant = errors.New("wire: unknown hub variant")

func (v Variant) String() string {
	switch v {
	case X1:
		return "X1"
	case X1S:
		return "X1S"
	case X2:
		return "X2"
	}
	return fmt.Sprintf("Variant(%d)", uint8(v))
}

// ParseHVER maps the HVER value of the hub's mDNS TXT record to a variant.
func ParseHVER(hver string) (Variant, error) {
	switch hver {
	case "1":
		return X1, nil
	case "2":
		return X1S, nil
	case "3":
		return X2, nil
	}
	return 0, fmt.Errorf("%w: HVER %q", ErrUnknownVariant, hver)
}

// VariantFromModel maps the model code of the hub's banner and discovery
// reply to a variant.
func VariantFromModel(code byte) (Variant, error) {
	switch code {
	case 0x01:
		return X1, nil
	case 0x02:
		return X1S, nil
	case 0x03:
		return X2, nil
	}
	return 0, fmt.Errorf("%w: model code 0x%02X", ErrUnknownVariant, code)
}

// Schema holds the record dimensions that differ between variants.
type Schema struct {
	// Labels is the encoding of device, command and macro labels.
	Labels TextEncoding
	// DeviceBodyLen is the device/activity record body, checksum included;
	// DeviceSlotLen each of its name, brand and tail slots.
	DeviceBodyLen int
	DeviceSlotLen int
	// CommandStride is one command record; CommandLabelLen its label slot.
	CommandStride   int
	CommandLabelLen int
	// MacroLabelLen is a macro record's label slot.
	MacroLabelLen int
	// InputEntryStride is one entry of the inputs page; InputLabelLen its
	// label slot.
	InputEntryStride int
	InputLabelLen    int
}

var (
	narrowSchema = Schema{
		Labels:           ASCII,
		DeviceBodyLen:    120,
		DeviceSlotLen:    30,
		CommandStride:    40,
		CommandLabelLen:  30,
		MacroLabelLen:    30,
		InputEntryStride: 27,
		InputLabelLen:    20,
	}
	wideSchema = Schema{
		Labels:           UTF16BE,
		DeviceBodyLen:    210,
		DeviceSlotLen:    60,
		CommandStride:    70,
		CommandLabelLen:  60,
		MacroLabelLen:    60,
		InputEntryStride: 48,
		InputLabelLen:    40,
	}
)

// Schema returns the variant's record dimensions.
func (v Variant) Schema() (Schema, error) {
	switch v {
	case X1:
		return narrowSchema, nil
	case X1S, X2:
		return wideSchema, nil
	}
	return Schema{}, fmt.Errorf("%w: %v", ErrUnknownVariant, v)
}
