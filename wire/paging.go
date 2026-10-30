package wire

import (
	"encoding/binary"
	"fmt"
)

const (
	// PageHeaderLen is the wrapper in front of each page of a paged write:
	// a marker byte and the big-endian page number.
	PageHeaderLen = 3
	// PageChunk is the body bytes one page carries.
	PageChunk = MaxPayload - PageHeaderLen - 5
)

// PageCount is the number of pages a body of n bytes is written in; an
// empty body still takes one page.
func PageCount(n int) int {
	return max(1, (n+PageChunk-1)/PageChunk)
}

// Pages splits body into the payloads of a paged write. Each starts with
// the marker (0x01 for most families; the command sequence number for
// family 0x0E) and its 1-based page number.
func Pages(marker byte, body []byte) [][]byte {
	count := PageCount(len(body))
	pages := make([][]byte, count)
	for i := range count {
		chunk := body[i*PageChunk : min((i+1)*PageChunk, len(body))]
		p := make([]byte, 0, PageHeaderLen+len(chunk))
		p = append(p, marker)
		p = binary.BigEndian.AppendUint16(p, uint16(i+1))
		pages[i] = append(p, chunk...)
	}
	return pages
}

// PageHeader is the wrapper of one page of a paged write.
type PageHeader struct {
	Marker byte
	Page   uint16
}

// SplitPage separates a paged write's payload into its wrapper and chunk.
// The chunk aliases payload.
func SplitPage(payload []byte) (PageHeader, []byte, error) {
	if len(payload) < PageHeaderLen {
		return PageHeader{}, nil, fmt.Errorf("wire: page of %d bytes has no header", len(payload))
	}
	return PageHeader{
		Marker: payload[0],
		Page:   binary.BigEndian.Uint16(payload[1:3]),
	}, payload[PageHeaderLen:], nil
}
