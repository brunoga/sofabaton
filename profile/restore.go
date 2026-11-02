package profile

import (
	"context"
	"fmt"

	"github.com/brunoga/sofabaton/hub"
)

// Restore makes the hub dst hold what src holds (a hub, or a dump served
// by dump.File.Open): src's configuration, read with Snapshot, is applied
// to dst with ApplyConfig, matching devices and activities by name. With
// prune, a match loses what src's profile lacks; dst's devices and
// activities that src lacks are kept, and reported. With dryRun nothing is
// written.
func Restore(ctx context.Context, src, dst *hub.Client, prune, dryRun bool, report func(string)) error {
	cfg, err := Snapshot(ctx, src, nil)
	if err != nil {
		return fmt.Errorf("profile: restore: reading the source: %w", err)
	}
	return ApplyConfig(ctx, dst, cfg, ApplyOptions{Prune: prune, DryRun: dryRun, Report: report})
}
