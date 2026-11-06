//go:build !linux

package main

import (
	"context"
	"errors"
	"net"

	"github.com/brunoga/sofabaton/sifli"
)

// bleDial is Linux's only (BlueZ) for now.
func bleDial(net.HardwareAddr) sifli.Dial {
	return func(context.Context) (sifli.Conn, error) {
		return nil, errors.New("updating a remote needs Bluetooth, which sbx has on Linux only for now")
	}
}
