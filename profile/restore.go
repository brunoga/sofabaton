package profile

import (
	"context"
	"fmt"

	"github.com/brunoga/sofabaton/hub"
)

// Restore makes the hub dst hold what src holds (a hub, or a dump served
// by dump.File.Open): src's configuration, read with Snapshot, is applied
// to dst with ApplyConfig and opts, matching devices and activities by name
// (opts.ByID is ignored). With opts.Prune, a match loses what src's profile
// lacks; dst's devices and activities that src lacks are kept, and
// reported, unless opts.Delete. With opts.Pair, Bluetooth devices src has
// and dst lacks are created and paired again.
func Restore(ctx context.Context, src, dst *hub.Client, opts ApplyOptions) error {
	cfg, err := Snapshot(ctx, src, nil)
	if err != nil {
		return fmt.Errorf("profile: restore: reading the source: %w", err)
	}
	opts.ByID = false
	return ApplyConfig(ctx, dst, cfg, opts)
}
