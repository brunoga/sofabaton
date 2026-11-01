package profile

import (
	"fmt"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/brunoga/sofabaton/hub"
	"github.com/brunoga/sofabaton/record"
)

// FavoriteList is an activity's favorites: commands of its devices
// ("Device: Command"), on the remote's list in this order, after the
// activity's macros. nil leaves the hub's favorites and their places as
// they are; favorites the list lacks stay unless pruned, so empty with
// prune removes them all.
//
//	favorites:
//	  - 'Receiver: Movie'
//	  - 'Projector: Picture Mode'
type FavoriteList []string

// IsZero keeps an empty list in the YAML: only nil is left out.
func (l FavoriteList) IsZero() bool { return l == nil }

// UnmarshalYAML reads a list of favorites; [] is an empty list, not nil.
func (l *FavoriteList) UnmarshalYAML(n *yaml.Node) error {
	var favs []string
	if err := n.Decode(&favs); err != nil {
		return err
	}
	if favs == nil {
		favs = []string{}
	}
	*l = favs
	return nil
}

// favorite is one of an activity's favorites on the hub: its keymap row,
// whose button is the favorite's id.
type favorite struct {
	ID    byte
	Press record.Press
}

// favorites are the activity's favorites on the hub, in keymap order:
// the rows that are not hard keys.
func (st *ActivityState) favorites() []favorite {
	var out []favorite
	for _, b := range st.Keymap {
		if !b.Button.IsHardKey() && b.Short.Device != 0 {
			out = append(out, favorite{ID: byte(b.Button), Press: b.Short})
		}
	}
	return out
}

// favKey names a favorite by its command, for the ids the hub gives new
// ones.
func favKey(p record.Press) string {
	return fmt.Sprintf("favorite\x00%d\x00%d", p.Device, p.Command)
}

// validateFavorites checks that the favorites are commands of the
// activity's devices, each once.
func (a *Activity) validateFavorites(members map[string]bool) []error {
	var errs []error
	seen := map[string]bool{}
	for _, f := range a.Favorites {
		dev, cmd, ok := strings.Cut(f, ": ")
		if !ok || cmd == "" || !members[strings.ToLower(dev)] {
			errs = append(errs, fmt.Errorf("profile: favorites: %q is not \"Device: Command\" of a device of the activity", f))
			continue
		}
		if key := strings.ToLower(f); seen[key] {
			errs = append(errs, fmt.Errorf("profile: favorites: %q appears twice", f))
		} else {
			seen[key] = true
		}
	}
	return errs
}

// planFavorites plans the favorites' removals and additions, and returns
// their entries on the remote's list in the profile's order, and the ids
// of the favorites it removes. The favorites already there keep their
// ids; new ones get theirs when added.
func planFavorites(a *Activity, st *ActivityState, r resolver, prune bool) (changes []ActivityChange, order []OrderEntry, removed []byte, err error) {
	if a.Favorites == nil {
		return nil, nil, nil, nil
	}
	act := st.Entity.ID
	have := map[string]favorite{}
	for _, f := range st.favorites() {
		if _, ok := have[favKey(f.Press)]; !ok {
			have[favKey(f.Press)] = f
		}
	}
	wanted := map[string]bool{}
	var adds []ActivityChange
	for _, name := range a.Favorites {
		devName, cmdName, _ := strings.Cut(name, ": ")
		dev, err := r.device(devName)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("%w (favorite %s)", err, name)
		}
		cmd, err := r.command(dev, cmdName)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("%w (favorite %s)", err, name)
		}
		if cmd.ID > hub.MaxFavoriteCommand {
			return nil, nil, nil, fmt.Errorf("profile: favorite %s: command id %d is above the %d the hub takes", name, cmd.ID, hub.MaxFavoriteCommand)
		}
		p := record.Press{Device: dev, Command: cmd.ID, Code: cmd.Code}
		key := favKey(p)
		wanted[key] = true
		if f, ok := have[key]; ok {
			order = append(order, OrderEntry{ID: f.ID})
			continue
		}
		adds = append(adds, ActivityChange{Kind: ActNewFavorite, Activity: act, Name: name,
			Binding: record.Binding{Entity: act, Short: p}})
		order = append(order, OrderEntry{Favorite: key})
	}
	if prune {
		for _, f := range st.favorites() {
			if !wanted[favKey(f.Press)] && !slices.Contains(removed, f.ID) {
				changes = append(changes, ActivityChange{Kind: ActRemoveFavorite, Activity: act, Favorite: f.ID,
					Name: st.Names[f.Press.Device] + ": " + st.commandLabel(f.Press)})
				removed = append(removed, f.ID)
			}
		}
	}
	// Removals first: they free ids for the additions.
	return append(changes, adds...), order, removed, nil
}

// commandLabel names a press's command, or its id.
func (st *ActivityState) commandLabel(p record.Press) string {
	for _, c := range st.Commands[p.Device] {
		if c.ID == p.Command {
			return c.Label
		}
	}
	return fmt.Sprintf("#%d", p.Command)
}
