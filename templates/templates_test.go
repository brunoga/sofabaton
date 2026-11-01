package templates

import (
	"testing"

	"github.com/brunoga/sofabaton/profile"
)

// Every template loads and makes a profile; the Aetherion's has every key
// of AWOL Vision's IR list.
func TestTemplates(t *testing.T) {
	names := Names()
	if len(names) == 0 {
		t.Fatal("no templates")
	}
	for _, name := range names {
		data, err := Read(name)
		if err != nil {
			t.Fatal(err)
		}
		if !profile.IsTemplate(data) {
			t.Errorf("%s: no template key", name)
		}
		d, err := profile.LoadTemplate(data)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if name == "ir/awol-vision/aetherion" && len(d.Commands) != 35 {
			t.Errorf("%s: %d commands, AWOL Vision's IR list has 35", name, len(d.Commands))
		}
		if d.ID != 0 {
			t.Errorf("%s: has an id", name)
		}
		out, err := profile.FromTemplate(data, name, "My Device")
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if profile.IsTemplate(out) {
			t.Errorf("%s: the profile made from it is still a template", name)
		}
	}
}
