package irsource

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/brunoga/sofabaton/record"
	"github.com/brunoga/sofabaton/sifli"
	"github.com/brunoga/sofabaton/wire"
)

// Firmware is a firmware the vendor's server offers, as the app asks it
// for its hub's and remotes' updates (app/checkFirmwareUpgrade, seen in
// the app's code and asked for our X2, 2026-11-06).
type Firmware struct {
	// Product is the server's name of what it is for: "x2" the X2 hub.
	Product string
	// Version is the firmware version it installs.
	Version int
	// Notes say what it changes, in the vendor's words.
	Notes string
	// URLs are its files: one image for a hub, several for a remote.
	URLs []string
	// Force is set for an update the app does not let users skip.
	Force bool
}

// Hub products and hardware versions, as the hubs announce them (mDNS
// HVER) and the app sends the server.
var hubProducts = map[wire.Variant]struct {
	name     string
	hardware int
}{
	wire.X1:  {"x1", 1},
	wire.X1S: {"x1s", 2},
	wire.X2:  {"x2", 3},
}

// HubFirmware is the update the server offers the hub of info, or nil if
// it has none newer than the hub's.
func (db SofabatonDB) HubFirmware(ctx context.Context, info *record.HubInfo) (*Firmware, error) {
	p, ok := hubProducts[info.Variant]
	if !ok {
		return nil, fmt.Errorf("irsource: no firmware updates are known for hub model %#x", info.Model)
	}
	return db.firmware(ctx, p.name, int(info.Firmware), p.hardware, hex.EncodeToString(info.Batch[:]))
}

// firmware asks the server for the update of product from version: nil
// if there is none (the server's code 1).
func (db SofabatonDB) firmware(ctx context.Context, product string, version, hardware int, batch string) (*Firmware, error) {
	var r struct {
		Product string `json:"product_name"`
		Version string `json:"software_code"`
		Notes   string `json:"cont"`
		URL     string `json:"url"`
		Force   int    `json:"is_force"`
	}
	err := db.post(ctx, "app/checkFirmwareUpgrade", map[string]any{
		"product_name": product, "software_code": version, "hardware_code": hardware, "production_time": batch,
	}, &r)
	var ce *catalogError
	if errors.As(err, &ce) && ce.code == 1 {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	v, err := strconv.Atoi(r.Version)
	if err != nil {
		return nil, fmt.Errorf("irsource: Sofabaton's firmware version %q", r.Version)
	}
	f := &Firmware{Product: r.Product, Version: v, Notes: r.Notes, Force: r.Force != 0}
	for _, u := range strings.Split(r.URL, "&&") {
		if u = strings.TrimSpace(u); u != "" {
			f.URLs = append(f.URLs, u)
		}
	}
	if f.Version <= version || len(f.URLs) == 0 {
		return nil, fmt.Errorf("irsource: Sofabaton offered %s firmware %d for %d, with %d files", product, f.Version, version, len(f.URLs))
	}
	return f, nil
}

// firmwareHost is where the server's firmware files are.
const firmwareHost = "sofabaton.s3.us-west-2.amazonaws.com"

// Download is the firmware's file at u, one of its URLs; only the
// vendor's file store is asked.
func (db SofabatonDB) Download(ctx context.Context, u string) ([]byte, error) {
	p, err := url.Parse(u)
	if err != nil || p.Scheme != "https" || p.Host != firmwareHost {
		return nil, fmt.Errorf("irsource: %q is not a file of Sofabaton's firmware store", u)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	return db.do(req)
}

// HubImage is the hub firmware's image, downloaded: the one file of a
// hub's update, which hub.Client.UpdateFirmware sends the hub with its
// MD5, for the hub to check.
func (db SofabatonDB) HubImage(ctx context.Context, f *Firmware) ([]byte, error) {
	if len(f.URLs) != 1 {
		return nil, fmt.Errorf("irsource: a hub firmware of %d files", len(f.URLs))
	}
	b, err := db.Download(ctx, f.URLs[0])
	if err != nil {
		return nil, err
	}
	if len(b) == 0 {
		return nil, errors.New("irsource: an empty firmware image")
	}
	return b, nil
}

// remoteProduct is the server's name of the X2 remote's firmware.
const remoteProduct = "remote2_release"

// RemoteFirmwares are the firmwares the server lists for the X2's remote.
func (db SofabatonDB) RemoteFirmwares(ctx context.Context) ([]*Firmware, error) {
	var rows []struct {
		Product string `json:"product_name"`
		Version string `json:"software_code"`
		Notes   string `json:"cont"`
		URL     string `json:"url"`
		Force   int    `json:"is_force"`
	}
	if err := db.post(ctx, "app/getFirmwares", map[string]any{"product_name": remoteProduct}, &rows); err != nil {
		return nil, err
	}
	var out []*Firmware
	for _, row := range rows {
		v, err := strconv.Atoi(row.Version)
		if err != nil {
			continue
		}
		f := &Firmware{Product: row.Product, Version: v, Notes: row.Notes, Force: row.Force != 0}
		for _, u := range strings.Split(row.URL, "&&") {
			if u = strings.TrimSpace(u); u != "" {
				f.URLs = append(f.URLs, u)
			}
		}
		out = append(out, f)
	}
	return out, nil
}

// RemoteFirmware is the newest firmware the server lists for the X2's
// remote, or nil if the remote's version is it.
func (db SofabatonDB) RemoteFirmware(ctx context.Context, r *record.RemoteInfo) (*Firmware, error) {
	all, err := db.RemoteFirmwares(ctx)
	if err != nil {
		return nil, err
	}
	var best *Firmware
	for _, f := range all {
		if f.Version > int(r.Firmware) && (best == nil || f.Version > best.Version) {
			best = f
		}
	}
	return best, nil
}

// remoteImages are the remote firmware's image files, by image id, as the
// app sends them after its ctrl_packet.bin.
var remoteImages = []string{"outcom_app.bin", "outcom_lcpu.bin", "outcom_lcpu_patch.bin", "outres.bin", "outfont.bin"}

// RemotePackage downloads the remote firmware's files: its control packet
// and its images, in the order sifli.Update sends them.
func (db SofabatonDB) RemotePackage(ctx context.Context, f *Firmware) (sifli.Package, error) {
	byName := map[string]string{}
	for _, u := range f.URLs {
		byName[path.Base(u)] = u
	}
	var p sifli.Package
	for i, name := range append([]string{"ctrl_packet.bin"}, remoteImages...) {
		u, ok := byName[name]
		if !ok {
			return sifli.Package{}, fmt.Errorf("irsource: the remote firmware has no %s", name)
		}
		b, err := db.Download(ctx, u)
		if err != nil {
			return sifli.Package{}, err
		}
		if len(b) == 0 {
			return sifli.Package{}, fmt.Errorf("irsource: an empty %s", name)
		}
		if i == 0 {
			p.Ctrl = b
		} else {
			p.Images = append(p.Images, b)
		}
	}
	return p, nil
}
