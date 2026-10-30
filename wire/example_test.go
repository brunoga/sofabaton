package wire_test

import (
	"bytes"
	"fmt"

	"github.com/brunoga/sofabaton/wire"
)

func ExampleDecoder() {
	// Two frames with a stray byte between them.
	stream := []byte{
		0xA5, 0x5A, 0x00, 0x3A, 0x39, // request activities
		0x42,
		0xA5, 0x5A, 0x01, 0x03, 0x00, 0x03, // status ack: accepted
	}
	d := wire.NewDecoder(bytes.NewReader(stream))
	for {
		f, err := d.Decode()
		if err != nil {
			break
		}
		fmt.Printf("%v %s [% x]\n", f.Opcode(), f.Family.Name(wire.FromHub), f.Payload)
	}
	fmt.Println("skipped", d.Skipped())
	// Output:
	// 0x003A 0x3A []
	// 0x0103 StatusAck [00]
	// skipped 1
}

func ExampleFrame_MarshalBinary() {
	f := wire.Frame{Family: wire.FamReqButtons, Payload: []byte{0x65, 0xFF}}
	b, _ := f.MarshalBinary()
	fmt.Printf("% X\n", b)
	// Output: A5 5A 02 3C 65 FF A1
}
