//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"tinygo.org/x/bluetooth"

	"github.com/brunoga/sofabaton/sifli"
)

// bleScanWait bounds finding the device before connecting: BlueZ
// connects only to devices it has seen.
const bleScanWait = 30 * time.Second

// bleDial connects to the device at addr over the computer's Bluetooth
// (BlueZ), on its SiFli data characteristic.
func bleDial(addr net.HardwareAddr) sifli.Dial {
	return func(ctx context.Context) (sifli.Conn, error) {
		a := bluetooth.DefaultAdapter
		if err := a.Enable(); err != nil {
			return nil, fmt.Errorf("bluetooth: %w", err)
		}
		mac, err := bluetooth.ParseMAC(addr.String())
		if err != nil {
			return nil, err
		}
		if err := bleFind(ctx, a, mac); err != nil {
			return nil, err
		}
		type result struct {
			d   bluetooth.Device
			err error
		}
		done := make(chan result, 1)
		go func() {
			// The library's Connect has no timeout of its own.
			d, err := a.Connect(bluetooth.Address{MACAddress: bluetooth.MACAddress{MAC: mac}}, bluetooth.ConnectionParams{})
			done <- result{d, err}
		}()
		var d bluetooth.Device
		select {
		case r := <-done:
			if r.err != nil {
				return nil, fmt.Errorf("bluetooth: connecting to %v: %w", addr, r.err)
			}
			d = r.d
		case <-ctx.Done():
			return nil, fmt.Errorf("bluetooth: connecting to %v: %w", addr, ctx.Err())
		}
		c, err := bleOpen(d)
		if err != nil {
			_ = d.Disconnect()
			return nil, err
		}
		return c, nil
	}
}

// bleFind scans until the device at mac advertises.
func bleFind(ctx context.Context, a *bluetooth.Adapter, mac bluetooth.MAC) error {
	ctx, cancel := context.WithTimeout(ctx, bleScanWait)
	defer cancel()
	found := make(chan struct{})
	var once sync.Once
	go func() {
		<-ctx.Done()
		_ = a.StopScan()
	}()
	err := a.Scan(func(a *bluetooth.Adapter, r bluetooth.ScanResult) {
		if r.Address.MAC == mac {
			once.Do(func() { close(found) })
			_ = a.StopScan()
		}
	})
	select {
	case <-found:
		return nil
	default:
	}
	if err != nil {
		return fmt.Errorf("bluetooth: scanning: %w", err)
	}
	return errors.New("bluetooth: the remote did not show up: wake it (press a key) and keep it near this computer")
}

// bleConn is the device's data characteristic.
type bleConn struct {
	d     bluetooth.Device
	char  bluetooth.DeviceCharacteristic
	notes chan []byte
	mtu   int
}

func bleOpen(d bluetooth.Device) (*bleConn, error) {
	svc, err := bluetooth.ParseUUID(sifli.ServiceUUID)
	if err != nil {
		return nil, err
	}
	data, err := bluetooth.ParseUUID(sifli.DataUUID)
	if err != nil {
		return nil, err
	}
	svcs, err := d.DiscoverServices([]bluetooth.UUID{svc})
	if err != nil || len(svcs) == 0 {
		return nil, fmt.Errorf("bluetooth: the remote has no update service: %v", err)
	}
	chars, err := svcs[0].DiscoverCharacteristics([]bluetooth.UUID{data})
	if err != nil || len(chars) == 0 {
		return nil, fmt.Errorf("bluetooth: the remote has no update characteristic: %v", err)
	}
	c := &bleConn{d: d, char: chars[0], notes: make(chan []byte, 256)}
	if err := c.char.EnableNotifications(func(b []byte) {
		c.notes <- append([]byte(nil), b...)
	}); err != nil {
		return nil, fmt.Errorf("bluetooth: notifications: %w", err)
	}
	mtu, err := c.char.GetMTU()
	if err != nil {
		mtu = 23 // the least; slow but sure
	}
	c.mtu = int(mtu)
	return c, nil
}

func (c *bleConn) Write(v []byte) error {
	_, err := c.char.WriteWithoutResponse(v)
	return err
}

func (c *bleConn) Notification(ctx context.Context) ([]byte, error) {
	select {
	case n := <-c.notes:
		return n, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (c *bleConn) MTU() int { return c.mtu }

func (c *bleConn) Close() error { return c.d.Disconnect() }
