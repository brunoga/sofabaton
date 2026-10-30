package record

import (
	"fmt"

	"github.com/brunoga/sofabaton/wire"
)

// CatalogRow is one row of a device (family 0x0B) or activity (family 0x3B)
// catalog burst, or of an activity's membership (family 0x6D, the member
// devices' records): one frame per entity.
type CatalogRow struct {
	// Index is the row's 1-based position in the burst, Total the burst's
	// row count; the burst is complete once rows 1..Total have arrived.
	Index  int
	Total  int
	Entity *Entity
}

// ParseCatalogRow decodes a catalog row's payload: a 3-byte page header
// (row index, then 00 01) and the entity body, whose first byte carries the
// row count instead of the write-form page marker.
func ParseCatalogRow(v wire.Variant, payload []byte) (CatalogRow, error) {
	h, body, err := wire.SplitPage(payload)
	if err != nil {
		return CatalogRow{}, fmt.Errorf("record: catalog row: %w", err)
	}
	e, err := ParseEntity(v, body)
	if err != nil {
		return CatalogRow{}, fmt.Errorf("record: catalog row: %w", err)
	}
	return CatalogRow{Index: int(h.Marker), Total: int(body[0]), Entity: e}, nil
}
