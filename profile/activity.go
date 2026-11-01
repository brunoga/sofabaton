package profile

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/brunoga/sofabaton/hub"
	"github.com/brunoga/sofabaton/record"
)

// Activity is an activity's profile: its devices with the input each is
// switched to, its keys, its power macros and its other macros.
// Keys and steps name commands as "Device: Command".
//
//	activity: Watch Blu-ray
//	id: 103
//	devices:
//	  - {device: Receiver, input: Input Blu-ray}
//	  - {device: Blu-ray}
//	buttons:
//	  Vol+: 'Receiver: Volume Up'
//	  Play: 'Blu-ray: Play'
//	power_on:
//	  - power on: Receiver
//	  - wait: 1
//	  - input: Receiver
//	power_off:
//	  - power off: Receiver
//	macros:
//	  - name: Movie Mode
//	    steps:
//	      - Receiver: Movie
//	      - wait: 2
type Activity struct {
	Name string `yaml:"activity"`
	// ID is the activity's id on the hub the profile belongs to.
	ID uint8 `yaml:"id,omitempty"`
	// Icon and Color are the activity's on the remote: an icon number
	// and a palette color (1-12). 0 leaves the hub's as they are.
	Icon    uint8    `yaml:"icon,omitempty"`
	Color   uint8    `yaml:"color,omitempty"`
	Devices []Member `yaml:"devices"`
	// Buttons bind the remote's keys to the devices' commands and macros
	// ("Device: Command") or to the activity's macros (by name).
	Buttons  Buttons         `yaml:"buttons,omitempty"`
	PowerOn  []Step          `yaml:"power_on,omitempty"`
	PowerOff []Step          `yaml:"power_off,omitempty"`
	Macros   []ActivityMacro `yaml:"macros,omitempty"`
}

// Member is a device of an activity and the input (a command of the
// device's inputs) the activity switches it to, if any.
type Member struct {
	Device string `yaml:"device"`
	Input  string `yaml:"input,omitempty"`
}

// ActivityMacro is one of an activity's macros, run from a key or from the
// remote's list.
type ActivityMacro struct {
	Name  string `yaml:"name"`
	Steps []Step `yaml:"steps"`
}

// StepKind is what a step does.
type StepKind int

const (
	// StepCommand sends a device's command.
	StepCommand StepKind = iota
	// StepWait pauses for Seconds.
	StepWait
	// StepPowerOn and StepPowerOff run the device's power macros.
	StepPowerOn
	StepPowerOff
	// StepInput switches the device to its input for the activity.
	StepInput
)

// Step is one step of a macro. In YAML it is a mapping of one key:
// "wait: 2", "power on: Receiver", "power off: Receiver",
// "input: Receiver" or "Receiver: Volume Up"; a command step can add
// "hold" (the hub's duration byte, meaning unknown).
type Step struct {
	Kind    StepKind
	Device  string
	Command string
	Seconds uint8
	Hold    uint8
}

var stepWords = map[string]StepKind{"wait": StepWait, "power on": StepPowerOn, "power off": StepPowerOff, "input": StepInput}

func (s Step) MarshalYAML() (any, error) {
	scalar := func(v string) *yaml.Node { return &yaml.Node{Kind: yaml.ScalarNode, Value: v} }
	n := &yaml.Node{Kind: yaml.MappingNode}
	for w, k := range stepWords {
		if k != s.Kind {
			continue
		}
		v := s.Device
		if k == StepWait {
			v = strconv.Itoa(int(s.Seconds))
		}
		n.Content = []*yaml.Node{scalar(w), scalar(v)}
		return n, nil
	}
	n.Content = []*yaml.Node{scalar(s.Device), scalar(s.Command)}
	if s.Hold != 0 {
		n.Content = append(n.Content, scalar("hold"), scalar(strconv.Itoa(int(s.Hold))))
	}
	return n, nil
}

func (s *Step) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.MappingNode || len(n.Content) < 2 {
		return fmt.Errorf("line %d: a step is a mapping such as \"wait: 1\" or \"Receiver: Volume Up\"", n.Line)
	}
	*s = Step{}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		if v.Kind != yaml.ScalarNode {
			return fmt.Errorf("line %d: %s takes a name or a number", v.Line, k.Value)
		}
		if i > 0 {
			if k.Value != "hold" || s.Kind != StepCommand {
				return fmt.Errorf("line %d: a step has one key (and a command step hold)", k.Line)
			}
			h, err := strconv.ParseUint(v.Value, 10, 8)
			if err != nil {
				return fmt.Errorf("line %d: hold %q is not 0-255", v.Line, v.Value)
			}
			s.Hold = uint8(h)
			continue
		}
		kind, ok := stepWords[strings.ToLower(k.Value)]
		switch {
		case !ok:
			s.Kind, s.Device, s.Command = StepCommand, k.Value, v.Value
		case kind == StepWait:
			sec, err := strconv.ParseUint(v.Value, 10, 8)
			if err != nil || sec == 0 || sec == 0xFF {
				return fmt.Errorf("line %d: wait %q is not 1-254 seconds", v.Line, v.Value)
			}
			s.Kind, s.Seconds = kind, uint8(sec)
		default:
			s.Kind, s.Device = kind, v.Value
		}
	}
	return nil
}

func (s Step) String() string {
	switch s.Kind {
	case StepWait:
		return fmt.Sprintf("wait %d", s.Seconds)
	case StepPowerOn:
		return s.Device + ": power on"
	case StepPowerOff:
		return s.Device + ": power off"
	case StepInput:
		return s.Device + ": input"
	}
	return s.Device + ": " + s.Command
}

// macroInput is the key of the step that switches a device to its input
// for the activity; the step's duration is the input's ordinal.
const macroInput = 0xC5

// ActivityState is what the hub holds for an activity and its devices.
type ActivityState struct {
	Entity   *record.Entity
	Members  []*record.Entity
	Keymap   []record.Binding
	Macros   []*record.Macro
	Commands map[byte][]*record.Command
	Inputs   map[byte]*record.Inputs
	// Order is the order of the remote's list of the activity's
	// macros (and favorites).
	Order []record.Place
	// DeviceMacros are the copies of devices' macros in all activities,
	// by device: an activity a device joins gets them.
	DeviceMacros map[byte][]*record.Macro
	// Names are the hub's devices and activities by id.
	//
	// fresh marks an activity CreateActivity has just made, which gets
	// the app's power macros when the profile has none.
	Names map[byte]string
	fresh bool
}

// ReadActivity fetches an activity's state from the hub.
func ReadActivity(ctx context.Context, c *hub.Client, activity byte) (*ActivityState, error) {
	st, acts, err := readDevices(ctx, c)
	if err != nil {
		return nil, err
	}
	for _, e := range acts {
		if e.ID == activity {
			st.Entity = e
		}
	}
	if st.Entity == nil {
		return nil, fmt.Errorf("profile: %w: activity %d", hub.ErrNotFound, activity)
	}
	if st.Members, err = c.Members(ctx, activity); err != nil && !errors.Is(err, hub.ErrNotFound) {
		return nil, err
	}
	if st.Keymap, err = c.Keymap(ctx, activity); err != nil && !errors.Is(err, hub.ErrNotFound) {
		return nil, err
	}
	if st.Macros, err = c.Macros(ctx, activity); err != nil && !errors.Is(err, hub.ErrNotFound) {
		return nil, err
	}
	if st.Order, err = c.Order(ctx, activity); err != nil && !errors.Is(err, hub.ErrNotFound) {
		return nil, err
	}
	return st, nil
}

// readDevices fetches the devices with their commands and inputs, which
// an activity's keys and steps name, and the activities with the copies
// of devices' macros in them.
func readDevices(ctx context.Context, c *hub.Client) (*ActivityState, []*record.Entity, error) {
	devs, err := c.Devices(ctx)
	if err != nil {
		return nil, nil, err
	}
	st := &ActivityState{Commands: map[byte][]*record.Command{}, Inputs: map[byte]*record.Inputs{}, Names: map[byte]string{}, DeviceMacros: map[byte][]*record.Macro{}}
	for _, d := range devs {
		st.Names[d.ID] = d.Name
		if st.Commands[d.ID], err = c.Commands(ctx, d.ID); err != nil && !errors.Is(err, hub.ErrNotFound) {
			return nil, nil, err
		}
		in, err := c.Inputs(ctx, d.ID)
		if err != nil && !errors.Is(err, hub.ErrNotFound) {
			return nil, nil, err
		}
		if in != nil {
			st.Inputs[d.ID] = in
		}
	}
	acts, err := c.Activities(ctx)
	if err != nil && !errors.Is(err, hub.ErrNotFound) {
		return nil, nil, err
	}
	for _, e := range acts {
		st.Names[e.ID] = e.Name
		macros, err := c.Macros(ctx, e.ID)
		if err != nil && !errors.Is(err, hub.ErrNotFound) {
			return nil, nil, err
		}
		for _, m := range macros {
			if m.Origin != 0 {
				st.DeviceMacros[m.Origin] = append(st.DeviceMacros[m.Origin], m)
			}
		}
	}
	return st, acts, nil
}

// ActivityFromHub builds an activity's profile from its state (see
// ReadActivity).
func ActivityFromHub(st *ActivityState) (*Activity, error) {
	a := &Activity{Name: st.Entity.Name, ID: st.Entity.ID, Icon: st.Entity.Icon, Color: st.Entity.ActivityColor()}
	device := func(id byte) string {
		if n, ok := st.Names[id]; ok {
			return n
		}
		return fmt.Sprintf("#%d", id)
	}
	command := func(dev, id byte) string {
		for _, c := range st.Commands[dev] {
			if c.ID == id {
				return c.Label
			}
		}
		return fmt.Sprintf("#%d", id)
	}
	macros := map[byte]*record.Macro{}
	for _, m := range st.Macros {
		macros[m.ID] = m
	}
	press := func(p record.Press) string {
		if p.Device == a.ID {
			if m, ok := macros[p.Command]; ok && m.Origin != 0 {
				return device(m.Origin) + ": " + m.Label // a device's macro
			} else if ok {
				return m.Label
			}
			return fmt.Sprintf("#%d", p.Command)
		}
		return device(p.Device) + ": " + command(p.Device, p.Command)
	}
	steps := func(m *record.Macro) []Step {
		var out []Step
		for _, s := range m.Steps {
			switch {
			case s.IsDelay():
				out = append(out, Step{Kind: StepWait, Seconds: s.Delay})
			case s.Command == record.MacroPowerOn:
				out = append(out, Step{Kind: StepPowerOn, Device: device(s.Device)})
			case s.Command == record.MacroPowerOff:
				out = append(out, Step{Kind: StepPowerOff, Device: device(s.Device)})
			case s.Command == macroInput:
				out = append(out, Step{Kind: StepInput, Device: device(s.Device)})
			default:
				out = append(out, Step{Kind: StepCommand, Device: device(s.Device), Command: command(s.Device, s.Command), Hold: s.Duration})
			}
		}
		return out
	}

	// A member's input is the one its input step's duration names, by
	// ordinal in the device's inputs.
	inputs := map[byte]string{}
	if m := macros[record.MacroPowerOn]; m != nil {
		for _, s := range m.Steps {
			if s.Command != macroInput || s.Duration == 0 || s.IsDelay() {
				continue
			}
			in := st.Inputs[s.Device]
			if in == nil {
				continue
			}
			for _, e := range in.Entries {
				if e.Ordinal == s.Duration {
					inputs[s.Device] = command(s.Device, e.Command)
				}
			}
		}
	}
	for _, m := range st.Members {
		a.Devices = append(a.Devices, Member{Device: m.Name, Input: inputs[m.ID]})
	}
	for _, b := range st.Keymap {
		k := Key{Short: press(b.Short)}
		if !b.Long.IsZero() {
			k.Long = press(b.Long)
		}
		if a.Buttons == nil {
			a.Buttons = Buttons{}
		}
		a.Buttons[b.Button.String()] = k
	}
	for _, m := range st.Macros {
		switch m.ID {
		case record.MacroPowerOn:
			a.PowerOn = steps(m)
		case record.MacroPowerOff:
			a.PowerOff = steps(m)
		default:
			if m.Origin != 0 {
				continue // the device's, in its profile
			}
			a.Macros = append(a.Macros, ActivityMacro{Name: m.Label, Steps: steps(m)})
		}
	}
	return a, nil
}

// Validate checks that the activity has a name and devices, and that its
// keys and steps name its devices and macros.
func (a *Activity) Validate() error {
	if a.Name == "" {
		return errors.New("profile: no activity name")
	}
	var errs []error
	if err := validateLook(a.Color); err != nil {
		errs = append(errs, err)
	}
	members := map[string]bool{}
	for _, m := range a.Devices {
		if m.Device == "" {
			errs = append(errs, errors.New("profile: a device of the activity has no name"))
		}
		key := strings.ToLower(m.Device)
		if members[key] {
			errs = append(errs, fmt.Errorf("profile: device %q appears twice", m.Device))
		}
		members[key] = true
	}
	if len(a.Devices) == 0 {
		errs = append(errs, errors.New("profile: the activity has no devices"))
	}
	seqs := map[string]bool{}
	for _, s := range a.Macros {
		key := strings.ToLower(s.Name)
		if s.Name == "" || seqs[key] {
			errs = append(errs, fmt.Errorf("profile: macro %q has no name or appears twice", s.Name))
		}
		seqs[key] = true
	}
	checkSteps := func(what string, steps []Step) {
		for i, s := range steps {
			if s.Kind != StepWait && !members[strings.ToLower(s.Device)] {
				errs = append(errs, fmt.Errorf("profile: %s, step %d: %q is not a device of the activity", what, i+1, s.Device))
			}
		}
	}
	checkSteps("power_on", a.PowerOn)
	checkSteps("power_off", a.PowerOff)
	for _, s := range a.Macros {
		checkSteps("macro "+s.Name, s.Steps)
	}
	for name, k := range a.Buttons {
		if _, err := record.ParseButton(name); err != nil {
			errs = append(errs, fmt.Errorf("profile: buttons: %w", err))
		}
		for _, p := range []string{k.Short, k.Long} {
			if p == "" || seqs[strings.ToLower(p)] {
				continue
			}
			dev, _, ok := strings.Cut(p, ": ")
			if !ok || !members[strings.ToLower(dev)] {
				errs = append(errs, fmt.Errorf("profile: buttons: %s runs %q, neither \"Device: Command\" of a device of the activity nor one of its macros", name, p))
			}
		}
	}
	return errors.Join(errs...)
}

// LoadActivity reads and validates an activity's profile.
func LoadActivity(r io.Reader) (*Activity, error) {
	dec := yaml.NewDecoder(r)
	dec.KnownFields(true)
	var a Activity
	if err := dec.Decode(&a); err != nil {
		return nil, fmt.Errorf("profile: %w", err)
	}
	if err := a.Validate(); err != nil {
		return nil, err
	}
	return &a, nil
}

// Encode writes the profile as YAML.
func (a *Activity) Encode(w io.Writer) error {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(a); err != nil {
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}
	_, err := w.Write(buf.Bytes())
	return err
}
