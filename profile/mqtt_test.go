package profile_test

import (
	"bytes"
	"context"
	"net/netip"
	"strings"
	"testing"

	"github.com/brunoga/sofabaton/hub"
	"github.com/brunoga/sofabaton/profile"
	"github.com/brunoga/sofabaton/record"
)

const mqttDevice = `device: ZZ HA
mqtt: true
commands:
  - name: Movie scene
  - name: Lights off
`

// A device of MQTT commands needs the hub's broker; it is made with each
// command's data its device and its own id, reads back as its profile,
// and a command added later gets its id the same way.
func TestCreateMQTT(t *testing.T) {
	ctx := context.Background()
	d, err := profile.Load(strings.NewReader(mqttDevice))
	if err != nil {
		t.Fatal(err)
	}
	c := emptyHub(t)
	if _, err := profile.Create(ctx, c, d, nil); err == nil || !strings.Contains(err.Error(), "broker") {
		t.Fatalf("without a broker: %v", err)
	}
	if err := c.SetMQTT(ctx, hub.MQTTBroker{Addr: netip.MustParseAddrPort("10.0.0.2:1883"), User: "a", Password: "b"}); err != nil {
		t.Fatal(err)
	}
	id, err := profile.Create(ctx, c, d, nil)
	if err != nil {
		t.Fatal(err)
	}
	st, err := profile.Read(ctx, c, id)
	if err != nil {
		t.Fatal(err)
	}
	if st.Entity.Class != record.ClassMQTT || len(st.Commands) != 2 || !bytes.Equal(st.Data[2], []byte{id, 2}) || st.Commands[1].Type != record.ClassMQTT {
		t.Fatalf("made %+v, %+v, data %v", st.Entity, st.Commands, st.Data)
	}
	if changes, _, err := profile.Plan(d, st, true); err != nil || len(changes) != 0 {
		t.Fatalf("after the create: %v %v", changes, err)
	}
	back, err := profile.FromHub(st.Entity, st)
	if err != nil {
		t.Fatal(err)
	}
	var y strings.Builder
	if err := back.Encode(&y); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(y.String(), "mqtt: true\n") || !strings.Contains(y.String(), "- name: Lights off\n") || strings.Contains(y.String(), "data:") {
		t.Fatalf("read back:\n%s", y.String())
	}

	d.Commands = append(d.Commands, profile.Command{Name: "Wake"})
	changes, _, err := profile.Plan(d, st, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := profile.Apply(ctx, c, changes, nil); err != nil {
		t.Fatal(err)
	}
	if st, err = profile.Read(ctx, c, id); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(st.Data[3], []byte{id, 3}) || st.Commands[2].Type != record.ClassMQTT {
		t.Fatalf("added %+v, data %v", st.Commands, st.Data)
	}
}

func TestMQTTValidate(t *testing.T) {
	for _, bad := range []string{
		"device: X\nmqtt: true\ncommands:\n  - {name: A, descriptor: 'P:NEC R:38400 D:0 S:206 F:1'}\n",
		"device: X\nmqtt: true\nroku: {address: 10.0.0.2}\ncommands:\n  - {name: A}\n",
		"device: X\nmqtt: true\ncommands:\n  - {name: A}\ninputs: [A]\n",
		"device: X\ncommands:\n  - {name: A}\n",
	} {
		if _, err := profile.Load(strings.NewReader(bad)); err == nil {
			t.Errorf("taken:\n%s", bad)
		}
	}
}
