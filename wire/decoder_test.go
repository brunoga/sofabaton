package wire

import (
	"bytes"
	"errors"
	"io"
	"testing"
	"testing/iotest"

	"github.com/brunoga/sofabaton/internal/testcorpus"
)

func mustMarshal(t testing.TB, f Frame) []byte {
	t.Helper()
	b, err := f.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func decodeAll(t *testing.T, r io.Reader) ([]Frame, *Decoder, error) {
	t.Helper()
	d := NewDecoder(r)
	var out []Frame
	for {
		f, err := d.Decode()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return out, d, nil
			}
			return out, d, err
		}
		out = append(out, f)
	}
}

func TestDecoderStream(t *testing.T) {
	a := Frame{Family: FamReqActivities}
	b := Frame{Family: FamStatusAck, Payload: []byte{0x00}}
	c := Frame{Family: FamCommandPage, Payload: bytes.Repeat([]byte{0xA5, 0x5A}, 40)}

	good := func() []byte {
		var s []byte
		for _, f := range []Frame{a, b, c} {
			s = append(s, mustMarshal(t, f)...)
		}
		return s
	}

	corruptB := good()
	corruptB[len(mustMarshal(t, a))+4] ^= 0xFF // b's payload; its checksum no longer matches

	tests := []struct {
		name    string
		stream  []byte
		want    []Frame
		skipped int64
	}{
		{"aligned", good(), []Frame{a, b, c}, 0},
		{"garbage first", append([]byte{0x00, 0xA5, 0x13, 0x5A}, good()...), []Frame{a, b, c}, 4},
		{"garbage between", bytes.Join([][]byte{mustMarshal(t, a), {0xA5, 0xA5, 0x00}, mustMarshal(t, b)}, nil), []Frame{a, b}, 3},
		{"corrupt frame dropped", corruptB, []Frame{a, c}, int64(b.Len())},
		{"trailing garbage", append(good(), 0x01, 0x02), []Frame{a, b, c}, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, r := range []struct {
				name string
				r    io.Reader
			}{
				{"whole", bytes.NewReader(tt.stream)},
				{"byte at a time", iotest.OneByteReader(bytes.NewReader(tt.stream))},
			} {
				got, d, err := decodeAll(t, r.r)
				if err != nil {
					t.Fatalf("%s: %v", r.name, err)
				}
				if !framesEqual(got, tt.want) {
					t.Fatalf("%s: got %v, want %v", r.name, got, tt.want)
				}
				if d.Skipped() != tt.skipped {
					t.Fatalf("%s: skipped %d, want %d", r.name, d.Skipped(), tt.skipped)
				}
			}
		})
	}
}

// A corrupted length byte claims more bytes than the stream has left; the
// frames after it must still be found.
func TestDecoderCorruptLengthNearEnd(t *testing.T) {
	a := Frame{Family: FamStatusAck, Payload: []byte{0x00}}
	stream := append([]byte{0xA5, 0x5A, 0xF0, 0x5D, 0x01}, mustMarshal(t, a)...)
	got, _, err := decodeAll(t, bytes.NewReader(stream))
	if err != nil {
		t.Fatal(err)
	}
	if !framesEqual(got, []Frame{a}) {
		t.Fatalf("got %v, want %v", got, []Frame{a})
	}
}

func TestDecoderUnexpectedEOF(t *testing.T) {
	full := mustMarshal(t, Frame{Family: FamStatusAck, Payload: []byte{0x00}})
	for n := 1; n < len(full); n++ {
		d := NewDecoder(bytes.NewReader(full[:n]))
		if _, err := d.Decode(); !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Errorf("cut at %d: error = %v, want io.ErrUnexpectedEOF", n, err)
		}
	}
	if _, err := NewDecoder(bytes.NewReader(nil)).Decode(); err != io.EOF {
		t.Fatalf("empty stream: error = %v, want io.EOF", err)
	}
}

func TestDecoderReadError(t *testing.T) {
	boom := errors.New("boom")
	d := NewDecoder(iotest.ErrReader(boom))
	if _, err := d.Decode(); !errors.Is(err, boom) {
		t.Fatalf("error = %v, want boom", err)
	}
}

func TestDecoderUpstreamStream(t *testing.T) {
	caps := testcorpus.Complete(t)
	var stream []byte
	for _, c := range caps {
		stream = append(stream, c.Bytes()...)
	}
	got, d, err := decodeAll(t, iotest.HalfReader(bytes.NewReader(stream)))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(caps) || d.Skipped() != 0 {
		t.Fatalf("decoded %d of %d frames, skipped %d bytes", len(got), len(caps), d.Skipped())
	}
}

func TestEncoder(t *testing.T) {
	var buf bytes.Buffer
	w := &countingWriter{w: &buf}
	e := NewEncoder(w)
	frames := []Frame{{Family: FamReqDevices}, {Family: FamReqButtons, Payload: []byte{0x65, 0xFF}}}
	for _, f := range frames {
		if err := e.Encode(f); err != nil {
			t.Fatal(err)
		}
	}
	if w.writes != len(frames) {
		t.Fatalf("%d writes for %d frames", w.writes, len(frames))
	}
	got, _, err := decodeAll(t, &buf)
	if err != nil || !framesEqual(got, frames) {
		t.Fatalf("decoded %v, %v", got, err)
	}
	if err := e.Encode(Frame{Payload: make([]byte, 300)}); !errors.Is(err, ErrPayloadTooLong) {
		t.Fatalf("oversized frame: %v", err)
	}
}

type countingWriter struct {
	w      io.Writer
	writes int
}

func (c *countingWriter) Write(p []byte) (int, error) {
	c.writes++
	return c.w.Write(p)
}

func framesEqual(a, b []Frame) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Family != b[i].Family || !bytes.Equal(a[i].Payload, b[i].Payload) {
			return false
		}
	}
	return true
}

// The decoder must never panic, and every frame it returns must be valid
// and present in the input.
func FuzzDecoder(f *testing.F) {
	caps := testcorpus.Complete(f)
	for i := 0; i+1 < len(caps); i += 2 {
		f.Add(append(caps[i].Bytes(), caps[i+1].Bytes()...))
	}
	f.Add([]byte{0xA5, 0x5A, 0xA5, 0x5A, 0x00})
	f.Fuzz(func(t *testing.T, data []byte) {
		d := NewDecoder(bytes.NewReader(data))
		var total int64
		for range len(data) + 1 {
			fr, err := d.Decode()
			if err != nil {
				if !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
					t.Fatalf("unexpected error %v", err)
				}
				break
			}
			b, err := fr.MarshalBinary()
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(data, b) {
				t.Fatalf("frame % x not in input", b)
			}
			total += int64(len(b))
		}
		if total+d.Skipped() > int64(len(data)) {
			t.Fatalf("accounted for %d+%d of %d bytes", total, d.Skipped(), len(data))
		}
	})
}
