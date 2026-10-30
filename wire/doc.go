// Package wire implements the framing layer of the Sofabaton hub protocol:
// frames and their checksum, a stream decoder that resynchronizes after
// corruption, opcode families, fixed-width label slots, the paging wrapper
// of multi-page writes, and the per-model wire dimensions.
//
// A frame is
//
//	A5 5A | len family | payload (len bytes) | sum8
//
// The opcode's high byte is the payload length, so the opcode carries one
// piece of information beyond the payload: the low byte, called the family
// here. A Frame is therefore a family and a payload; its opcode is derived.
//
// Everything in this package is pure: no I/O beyond the io.Reader and
// io.Writer it is handed, no goroutines, no global state.
package wire
