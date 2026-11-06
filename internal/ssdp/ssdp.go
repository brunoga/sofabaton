// Package ssdp finds devices on the local network by SSDP: an M-SEARCH
// sent to the multicast group, and the unicast answers.
package ssdp

import (
	"bufio"
	"bytes"
	"context"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"
)

// Response is a device's answer to a search.
type Response struct {
	Addr netip.Addr
	// Location is the URL of the device's description, USN its unique
	// name, ST the search target it answered.
	Location, USN, ST, Server string
}

var group = &net.UDPAddr{IP: net.IPv4(239, 255, 255, 250), Port: 1900}

// Search sends an M-SEARCH for st (such as "roku:ecp") and collects the
// answers until ctx is done, sending again each second. One answer is
// kept for each USN.
func Search(ctx context.Context, st string) ([]Response, error) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{})
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Close() }()
	msg := []byte("M-SEARCH * HTTP/1.1\r\nHOST: 239.255.255.250:1900\r\nMAN: \"ssdp:discover\"\r\nMX: 1\r\nST: " + st + "\r\n\r\n")
	stop := context.AfterFunc(ctx, func() { _ = conn.SetReadDeadline(time.Now()) })
	defer stop()
	var out []Response
	seen := map[string]bool{}
	next := time.Now()
	buf := make([]byte, 2048)
	for ctx.Err() == nil {
		if time.Now().After(next) {
			if _, err := conn.WriteToUDP(msg, group); err != nil {
				return out, err
			}
			next = time.Now().Add(time.Second)
		}
		_ = conn.SetReadDeadline(next)
		n, from, err := conn.ReadFromUDPAddrPort(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			if ctx.Err() != nil {
				break
			}
			return out, err
		}
		r, ok := parse(buf[:n], from.Addr().Unmap())
		if !ok || seen[r.USN] || (st != "ssdp:all" && !strings.EqualFold(r.ST, st)) {
			continue
		}
		seen[r.USN] = true
		out = append(out, r)
	}
	return out, nil
}

// parse reads an answer: an HTTP 200 response of headers.
func parse(b []byte, from netip.Addr) (Response, bool) {
	resp, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(b)), nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		return Response{}, false
	}
	_ = resp.Body.Close()
	r := Response{Addr: from, Location: resp.Header.Get("Location"), USN: resp.Header.Get("Usn"), ST: resp.Header.Get("St"), Server: resp.Header.Get("Server")}
	if r.USN == "" {
		r.USN = from.String() + " " + r.Location
	}
	return r, true
}
