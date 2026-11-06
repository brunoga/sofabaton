package irsource

import (
	"context"
	"testing"

	"github.com/brunoga/sofabaton/record"
	"github.com/brunoga/sofabaton/wire"
)

func TestHubFirmware(t *testing.T) {
	serveSofabaton(t)
	ctx := context.Background()
	db := SofabatonDB{Key: testKey}
	info := &record.HubInfo{Variant: wire.X2, Model: 0x03, Firmware: 8, Batch: [4]byte{0x20, 0x22, 0x11, 0x20}}
	f, err := db.HubFirmware(ctx, info)
	if err != nil || f == nil || f.Version != 9 || f.Product != "x2" || len(f.URLs) != 1 || f.Force {
		t.Fatalf("%+v %v", f, err)
	}
	info.Firmware = 9
	if f, err := db.HubFirmware(ctx, info); err != nil || f != nil {
		t.Errorf("from 9: %+v %v", f, err)
	}
	for _, u := range []string{"http://sofabaton.s3.us-west-2.amazonaws.com/x.bin", "https://example.com/x.bin", "https://sofabaton.s3.us-west-2.amazonaws.com.evil/x"} {
		if _, err := db.Download(ctx, u); err == nil {
			t.Errorf("%s downloaded", u)
		}
	}
}

func TestRemoteFirmware(t *testing.T) {
	serveSofabaton(t)
	ctx := context.Background()
	db := SofabatonDB{Key: testKey}
	f, err := db.RemoteFirmware(ctx, &record.RemoteInfo{Firmware: 8})
	if err != nil || f == nil || f.Version != 9 || len(f.URLs) != 6 || f.Notes != "Remote 9" {
		t.Fatalf("%+v %v", f, err)
	}
	if f, err := db.RemoteFirmware(ctx, &record.RemoteInfo{Firmware: 9}); err != nil || f != nil {
		t.Errorf("from 9: %+v %v", f, err)
	}
}
