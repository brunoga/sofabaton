// Package capture records hub traffic as JSON lines: one entry per frame,
// with its direction, link and bytes, and free-text notes that mark what
// was being done while it was recorded.
package capture

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/brunoga/sofabaton/wire"
)

// Entry is one line of a capture: a frame or a note.
type Entry struct {
	Time time.Time `json:"t"`
	// Link names the connection the frame crossed ("app" for the app's
	// side of a proxy, "hub" for the hub's, "udp" for discovery).
	Link string         `json:"link,omitempty"`
	Dir  wire.Direction `json:"dir,omitempty"`
	// Hex is the whole frame. Family and Name are for reading the file
	// and are not used when decoding it.
	Hex    string `json:"hex,omitempty"`
	Family string `json:"family,omitempty"`
	Name   string `json:"name,omitempty"`
	Note   string `json:"note,omitempty"`
}

// IsNote reports whether the entry is a note rather than a frame.
func (e Entry) IsNote() bool { return e.Hex == "" }

// Frame decodes the entry's frame.
func (e Entry) Frame() (wire.Frame, error) {
	raw, err := hex.DecodeString(e.Hex)
	if err != nil {
		return wire.Frame{}, fmt.Errorf("capture: %w", err)
	}
	var f wire.Frame
	if err := f.UnmarshalBinary(raw); err != nil {
		return wire.Frame{}, fmt.Errorf("capture: %w", err)
	}
	return f, nil
}

// Writer appends entries to a stream. It is safe for concurrent use.
type Writer struct {
	mu  sync.Mutex
	w   *bufio.Writer
	enc *json.Encoder
	now func() time.Time
}

// NewWriter returns a writer appending to w.
func NewWriter(w io.Writer) *Writer {
	bw := bufio.NewWriter(w)
	return &Writer{w: bw, enc: json.NewEncoder(bw), now: time.Now}
}

// Frame records a frame that crossed link in direction dir.
func (w *Writer) Frame(link string, dir wire.Direction, f wire.Frame) error {
	raw, err := f.MarshalBinary()
	if err != nil {
		return err
	}
	return w.write(Entry{
		Link: link, Dir: dir, Hex: hex.EncodeToString(raw),
		Family: f.Family.String(), Name: f.Family.Name(dir),
	})
}

// Note records a note.
func (w *Writer) Note(text string) error { return w.write(Entry{Note: text}) }

func (w *Writer) write(e Entry) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	e.Time = w.now()
	if err := w.enc.Encode(e); err != nil {
		return err
	}
	return w.w.Flush()
}

// Read returns the entries of a capture.
func Read(r io.Reader) ([]Entry, error) {
	var out []Entry
	dec := json.NewDecoder(r)
	for {
		var e Entry
		if err := dec.Decode(&e); err == io.EOF {
			return out, nil
		} else if err != nil {
			return out, fmt.Errorf("capture: entry %d: %w", len(out)+1, err)
		}
		out = append(out, e)
	}
}
