package wire

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// ErrIncomplete reports a paged reply whose pages have not all arrived.
var ErrIncomplete = errors.New("wire: paged reply incomplete")

// Assembler collects the pages of a paged reply (command lists, keymaps,
// macros, …) into one body. Each page's payload is the page wrapper
// (marker, page number) and a chunk; page 1's chunk starts the body with a
// marker byte and the reply's page count. Records may span pages, so they
// are decoded from the body, never from single pages.
type Assembler struct {
	chunks map[uint16][]byte
	total  int
}

// Add adds one page's payload and reports whether every page has arrived.
// A page that arrives twice replaces the earlier copy.
func (a *Assembler) Add(payload []byte) (done bool, err error) {
	h, chunk, err := SplitPage(payload)
	if err != nil {
		return false, err
	}
	if h.Page == 0 {
		return false, errors.New("wire: page number 0")
	}
	if a.chunks == nil {
		a.chunks = make(map[uint16][]byte)
	}
	if h.Page == 1 {
		if len(chunk) < 3 {
			return false, fmt.Errorf("wire: first page of %d bytes has no body header", len(chunk))
		}
		total := int(binary.BigEndian.Uint16(chunk[1:3]))
		if total == 0 {
			return false, errors.New("wire: paged reply of 0 pages")
		}
		a.total = total
	}
	if a.total > 0 && int(h.Page) > a.total {
		return false, fmt.Errorf("wire: page %d of %d", h.Page, a.total)
	}
	a.chunks[h.Page] = append([]byte(nil), chunk...)
	return a.Done(), nil
}

// Done reports whether every page has arrived.
func (a *Assembler) Done() bool {
	if a.total == 0 {
		return false
	}
	for p := 1; p <= a.total; p++ {
		if _, ok := a.chunks[uint16(p)]; !ok {
			return false
		}
	}
	return true
}

// Pages is the reply's page count, 0 until page 1 has arrived.
func (a *Assembler) Pages() int { return a.total }

// Body returns the chunks joined in page order: the body, starting with its
// marker and page count.
func (a *Assembler) Body() ([]byte, error) {
	if !a.Done() {
		return nil, ErrIncomplete
	}
	var body []byte
	for p := 1; p <= a.total; p++ {
		body = append(body, a.chunks[uint16(p)]...)
	}
	return body, nil
}

// Reset empties the assembler for the next reply.
func (a *Assembler) Reset() {
	clear(a.chunks)
	a.total = 0
}
