package hub

import (
	"context"
	"fmt"

	"github.com/brunoga/sofabaton/record"
	"github.com/brunoga/sofabaton/session"
	"github.com/brunoga/sofabaton/wire"
)

// Remotes are the remotes the X2 hub knows: one RemoteStatus frame each
// (our X2 has one; the count and index fields are read as such).
func (c *Client) Remotes(ctx context.Context) ([]*record.RemoteInfo, error) {
	var out []*record.RemoteInfo
	collect := session.CollectorFunc(func(f wire.Frame) (bool, bool, error) {
		if f.Family != wire.FamRemoteStatus {
			return false, false, nil
		}
		r, err := record.ParseRemoteInfo(f.Payload)
		if err != nil {
			return true, true, err
		}
		out = append(out, r)
		return true, r.Index >= r.Count, nil
	})
	ctx, cancel := c.bound(ctx)
	defer cancel()
	if err := c.s.Exchange(ctx, wire.Frame{Family: wire.FamReqRemoteList, Payload: []byte{0}}, collect); err != nil {
		return nil, fmt.Errorf("hub: remotes: %w", err)
	}
	return out, nil
}
