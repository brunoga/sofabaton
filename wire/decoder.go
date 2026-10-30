package wire

import (
	"bufio"
	"errors"
	"io"
)

// Decoder reads frames from a byte stream. Bytes that do not form a valid
// frame are skipped: on a bad checksum it moves one byte forward and looks
// for the next sync, so a corrupted frame costs that frame only.
type Decoder struct {
	r       *bufio.Reader
	skipped int64
}

// NewDecoder returns a decoder reading from r.
func NewDecoder(r io.Reader) *Decoder {
	return &Decoder{r: bufio.NewReaderSize(r, 4*MaxFrameLen)}
}

// Skipped is the number of bytes skipped so far because they were not part
// of a valid frame.
func (d *Decoder) Skipped() int64 { return d.skipped }

// Decode returns the next valid frame. It returns io.EOF when the stream
// ends between frames and io.ErrUnexpectedEOF when it ends inside one.
func (d *Decoder) Decode() (Frame, error) {
	for {
		head, err := d.r.Peek(2)
		if err != nil {
			return Frame{}, d.eof(len(head), err, len(head) == 1 && head[0] == Sync0)
		}
		if head[0] != Sync0 || head[1] != Sync1 {
			d.skip()
			continue
		}

		hdr, err := d.r.Peek(HeaderLen)
		if err != nil {
			return Frame{}, d.eof(len(hdr), err, true)
		}
		n := Overhead + int(hdr[2])
		buf, err := d.r.Peek(n)
		if err != nil {
			if errors.Is(err, io.EOF) {
				// The length may be corrupt rather than the stream cut
				// short: look for a frame in what is left.
				if fr, ok := d.salvage(buf); ok {
					return fr, nil
				}
			}
			return Frame{}, d.eof(len(buf), err, true)
		}

		fr, _, err := Parse(buf)
		if err != nil {
			d.skip()
			continue
		}
		if _, err := d.r.Discard(n); err != nil {
			return Frame{}, err
		}
		return fr, nil
	}
}

// salvage looks for a valid frame after the first byte of buf, the tail of
// the stream, discarding the bytes before it.
func (d *Decoder) salvage(buf []byte) (Frame, bool) {
	for i := 1; i+1 < len(buf); i++ {
		if buf[i] != Sync0 || buf[i+1] != Sync1 {
			continue
		}
		fr, n, err := Parse(buf[i:])
		if err != nil {
			continue
		}
		d.skipped += int64(i)
		if _, err := d.r.Discard(i + n); err != nil {
			return Frame{}, false
		}
		return fr, true
	}
	return Frame{}, false
}

func (d *Decoder) skip() {
	if _, err := d.r.Discard(1); err == nil {
		d.skipped++
	}
}

// eof maps a short read to io.EOF between frames and io.ErrUnexpectedEOF
// inside one. Other errors pass through.
func (d *Decoder) eof(have int, err error, inFrame bool) error {
	if !errors.Is(err, io.EOF) {
		return err
	}
	if have > 0 && !inFrame {
		d.skipped += int64(have)
		_, _ = d.r.Discard(have)
		return io.EOF
	}
	if have == 0 {
		return io.EOF
	}
	d.skipped += int64(have)
	_, _ = d.r.Discard(have)
	return io.ErrUnexpectedEOF
}

// Encoder writes frames to a stream, one Write per frame.
type Encoder struct {
	w   io.Writer
	buf []byte
}

// NewEncoder returns an encoder writing to w.
func NewEncoder(w io.Writer) *Encoder {
	return &Encoder{w: w, buf: make([]byte, 0, MaxFrameLen)}
}

// Encode writes f.
func (e *Encoder) Encode(f Frame) error {
	b, err := f.AppendBinary(e.buf[:0])
	if err != nil {
		return err
	}
	e.buf = b
	_, err = e.w.Write(b)
	return err
}
