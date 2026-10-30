package wire

import "fmt"

// Burst collects a reply made of several records, each a paged body of
// its own: catalog rows, macros. The page wrapper's marker is the record's
// 1-based index and the first byte of each body the number of records.
// Replies with a single record (command lists, keymaps) are bursts of one.
type Burst struct {
	records map[byte]*Assembler
	total   int
}

// Add adds one page's payload and reports whether every page of every
// record has arrived.
func (b *Burst) Add(payload []byte) (done bool, err error) {
	h, chunk, err := SplitPage(payload)
	if err != nil {
		return false, err
	}
	if h.Marker == 0 {
		return false, fmt.Errorf("wire: record index 0")
	}
	if h.Page == 1 && len(chunk) > 0 {
		total := int(chunk[0])
		if total == 0 || int(h.Marker) > total {
			return false, fmt.Errorf("wire: record %d of %d", h.Marker, total)
		}
		if b.total != 0 && b.total != total {
			return false, fmt.Errorf("wire: record %d says %d records, earlier ones %d", h.Marker, total, b.total)
		}
		b.total = total
	}
	if b.records == nil {
		b.records = make(map[byte]*Assembler)
	}
	a := b.records[h.Marker]
	if a == nil {
		a = &Assembler{}
		b.records[h.Marker] = a
	}
	if _, err := a.Add(payload); err != nil {
		return false, err
	}
	return b.Done(), nil
}

// Done reports whether every record is complete.
func (b *Burst) Done() bool {
	if b.total == 0 {
		return false
	}
	for i := 1; i <= b.total; i++ {
		if a := b.records[byte(i)]; a == nil || !a.Done() {
			return false
		}
	}
	return true
}

// Len is the number of records, 0 until a record's first page arrived.
func (b *Burst) Len() int { return b.total }

// Records returns the records' bodies in index order.
func (b *Burst) Records() ([][]byte, error) {
	if !b.Done() {
		return nil, ErrIncomplete
	}
	out := make([][]byte, b.total)
	for i := range out {
		body, err := b.records[byte(i+1)].Body()
		if err != nil {
			return nil, err
		}
		out[i] = body
	}
	return out, nil
}

// Reset empties the burst for the next reply.
func (b *Burst) Reset() {
	clear(b.records)
	b.total = 0
}
