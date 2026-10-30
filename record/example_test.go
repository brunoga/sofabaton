package record_test

import (
	"fmt"

	"github.com/brunoga/sofabaton/record"
	"github.com/brunoga/sofabaton/wire"
)

// A new IR device for an X2, as a create write's payload.
func ExampleNewEntity() {
	e, err := record.NewEntity(wire.X2)
	if err != nil {
		panic(err)
	}
	e.Name, e.Brand = "Projector", "Epson"
	e.Class, e.Type, e.Icon = record.ClassIR, 2, 1
	if err := e.SetDeviceTail(record.DeviceTail{
		HasPollTime: true,
		PowerStyle:  1,
		HasExtra:    true,
		Extra:       [3]byte{0, 0, byte(record.EmitHub | record.EmitPort1 | record.EmitPort2)},
	}); err != nil {
		panic(err)
	}
	p, err := e.WritePayload()
	if err != nil {
		panic(err)
	}
	fmt.Println(len(p), "bytes; id", fmt.Sprintf("%#x", p[3+4]))
	// Output: 213 bytes; id 0xff
}
