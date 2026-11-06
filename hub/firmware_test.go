package hub_test

import (
	"context"
	"crypto/md5"
	"testing"

	"github.com/brunoga/sofabaton/hub"
	"github.com/brunoga/sofabaton/hubsim"
	"github.com/brunoga/sofabaton/record"
)

// An image goes to the hub as the app sent our X2 its 8 to 9 update:
// its size and MD5, then 236-byte chunks numbered from 1.
func TestUpdateFirmware(t *testing.T) {
	ctx := context.Background()
	m := &hubsim.Model{Info: &record.HubInfo{Model: 0x03, Firmware: 9, Name: "X2 HUB"}}
	c, err := hub.Open(ctx, hubsim.Pipe(hubsim.FromModel(m)), hub.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	image := make([]byte, 51044) // the size of the 8 to 9 image: 217 chunks
	for i := range image {
		image[i] = byte(i * 7)
	}
	var last, total int
	if err := c.UpdateFirmware(ctx, image, func(done, n int) { last, total = done, n }); err != nil {
		t.Fatal(err)
	}
	if string(m.Firmware) != string(image) || last != 217 || total != 217 {
		t.Fatalf("took %d bytes, progress %d of %d", len(m.Firmware), last, total)
	}

	start := record.FirmwareStart(image)
	sum := md5.Sum(image)
	if len(start) != 26 || string(start[:10]) != "\x00\x00\x01\x00\x00\x00\x00\x00\xc7\x64" || string(start[10:]) != string(sum[:]) {
		t.Errorf("start % x", start)
	}
	chunks, _ := record.FirmwareChunks(image)
	if len(chunks[0]) != 9+236 || string(chunks[216][:9]) != "\x00\x00\x01\x00\x00\x00\x01\x00\xd9" || len(chunks[216]) != 9+68 {
		t.Errorf("chunks % x … % x", chunks[0][:9], chunks[216][:9])
	}
}
