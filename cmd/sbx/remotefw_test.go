package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"net"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/brunoga/sofabaton/hub"
	"github.com/brunoga/sofabaton/hubsim"
	"github.com/brunoga/sofabaton/irsource"
	"github.com/brunoga/sofabaton/record"
	"github.com/brunoga/sofabaton/sifli"
)

// fakeRemote answers an update as our remote did, and keeps what it got.
type fakeRemote struct {
	buf    []byte
	out    [][]byte
	slices int
	want   int // the image's slices
	images int
	ctrl   int
}

func (r *fakeRemote) MTU() int     { return 247 }
func (r *fakeRemote) Close() error { return nil }

func (r *fakeRemote) Notification(ctx context.Context) ([]byte, error) {
	if len(r.out) == 0 {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	n := r.out[0]
	r.out = r.out[1:]
	return n, nil
}

func (r *fakeRemote) reply(id int, hexPayload string) {
	p, _ := hex.DecodeString(hexPayload)
	msg := binary.LittleEndian.AppendUint16(nil, uint16(id))
	msg = binary.LittleEndian.AppendUint16(msg, uint16(len(p)))
	msg = append(msg, p...)
	r.out = append(r.out, append([]byte{1, 0, byte(len(msg)), byte(len(msg) >> 8)}, msg...))
}

func (r *fakeRemote) Write(f []byte) error {
	if f[1] == 0 || f[1] == 1 {
		r.buf = append([]byte(nil), f[4:]...)
	} else {
		r.buf = append(r.buf, f[2:]...)
	}
	if f[1] == 1 || f[1] == 2 {
		return nil
	}
	id, p := binary.LittleEndian.Uint16(r.buf), r.buf[4:]
	switch id {
	case 40:
		r.reply(41, "000004000100")
	case 43:
		r.reply(44, "0000010001140000000000000000001024020000")
	case 45:
		r.reply(46, "0000000000000000")
	case 47:
		if p[0] == 2 {
			r.ctrl = len(p) - 2
		} else {
			r.images++
			r.slices, r.want = 0, int(binary.LittleEndian.Uint32(p[6:]))
		}
		r.reply(48, "0000")
	case 49:
		r.slices++
		if r.slices%20 == 0 || r.slices == r.want {
			r.reply(50, "0000000000000000")
		}
	case 51:
		r.reply(52, "0000")
	case 53:
		r.reply(54, "0000")
	}
	return nil
}

// sbx firmware -remote reads the remote from the hub, finds its firmware
// on the server and sends it over Bluetooth.
func TestRemoteFirmware(t *testing.T) {
	status, _ := hex.DecodeString("010100000800042025061008000100faba25000f4e52656d6f7465203100000000000000000000000000000000000000000000")
	m := &hubsim.Model{Info: &record.HubInfo{Marker: 0x64, Model: 0x03, Firmware: 9, Name: "X2 HUB"}, Remotes: [][]byte{status}}
	prevDial, prevFW, prevPkg, prevBLE := dialHub, remoteFirmwares, remotePackage, remoteDial
	t.Cleanup(func() { dialHub, remoteFirmwares, remotePackage, remoteDial = prevDial, prevFW, prevPkg, prevBLE })
	dialHub = func(ctx context.Context, _ netip.Addr, opts hub.Options) (*hub.Client, error) {
		return hub.Open(ctx, hubsim.Pipe(hubsim.FromModel(m)), opts)
	}
	remoteFirmwares = func(context.Context, string) ([]*irsource.Firmware, error) {
		return []*irsource.Firmware{{Version: 9, Notes: "Faster"}, {Version: 8}}, nil
	}
	pkg := sifli.Package{Ctrl: make([]byte, 100), Images: [][]byte{make([]byte, 548*25), make([]byte, 600)}}
	remotePackage = func(_ context.Context, _ string, f *irsource.Firmware) (sifli.Package, error) {
		if f.Version != 9 {
			t.Errorf("version %d", f.Version)
		}
		return pkg, nil
	}
	fake := &fakeRemote{}
	remoteDial = func(addr net.HardwareAddr) sifli.Dial {
		if addr.String() != "fa:ba:25:00:0f:4e" {
			t.Errorf("dialled %v", addr)
		}
		return func(context.Context) (sifli.Conn, error) { return fake, nil }
	}
	var out, errs bytes.Buffer
	sbx := func(stdin string, args ...string) error {
		out.Reset()
		return run(context.Background(), strings.NewReader(stdin), append([]string{"firmware", "-hub", "192.0.2.1", "-key", "K", "-remote"}, args...), &out, &errs)
	}
	if err := sbx(""); err != nil || !strings.Contains(out.String(), "remote Remote 1 (fa:ba:25:00:0f:4e): firmware 8") || !strings.Contains(out.String(), "firmware 9 is available: Faster") {
		t.Fatalf("check: %v\n%s", err, out.String())
	}
	sifli.RestartPause = 0
	t.Cleanup(func() { sifli.RestartPause = 5 * time.Second })
	if err := sbx("y\n", "-update"); err != nil || fake.ctrl != 100 || fake.images != 2 || !strings.Contains(out.String(), "took firmware 9") {
		t.Fatalf("update: %v, ctrl %d images %d\n%s", err, fake.ctrl, fake.images, out.String())
	}
	m.Remotes[0][11] = 9
	if err := sbx("", "-reinstall", "-y"); err != nil || !strings.Contains(out.String(), "took firmware 9") {
		t.Fatalf("reinstall: %v\n%s", err, out.String())
	}
}
