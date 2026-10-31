# Connection and discovery

Reference: upstream `docs/protocol/connection-flow.md`, `hub-versions.md`.

## Outline (`transport.Dialer`, `transport.Discoverer`)

1. Discovery `[X2✓]`: mDNS (X2 observed advertising `_sofabaton_hub._udp.local.`;
   TXT carries `HVER`: 1=X1, 2=X1S, 3=X2) or UDP broadcast `NOTIFY_ME`.
2. `[X2✓]` Client sends `CALL_ME` (UDP, opcode `0x0CC3`, 12-byte payload: 6 reserved
   bytes, a local IPv4, a big-endian listen port, e.g. 8200) `[ref]`.
   The hub appears to use the UDP source address, not the IP field `[ref]`.
3. **The hub connects back** over TCP to the client's listen port `[ref]`.
4. Banner/identity exchange, then catalog requests `[ref]`.

## mDNS advertisement `[X2✓]`

Our X2 (2026-10-30, `avahi-browse`):

```
instance X1-HUB-962c08 · type _sofabaton_hub._udp.local · host X1-HUB-962c08.local
192.168.1.244 port 8102 · TXT HVER=3 NO=20221120 AVER=8 NAME=X2 HUB MAC=80f1b2962c08
```

The instance is `X1-HUB-` and the last three MAC bytes even on the X2; `NO` is
the production batch. The hub answers one-shot (legacy unicast) queries,
which `internal/mdns.Browse` uses. Implementation notes for anything that
advertises in the hub's place:

- Two sockets of one process on UDP 5353 share its traffic (each gets
  part, not all), so browse from an ephemeral port and leave 5353 to the
  responder.
- Go's `net.ListenMulticastUDP` turns multicast loopback off: answers then
  reach the LAN but not avahi or other browsers on the same host. Turn it
  back on.

## Single client

The hub serves one client at a time `[ref]`. To keep the Android app usable,
we run a proxy that advertises itself as the hub (mirrored mDNS name/TXT),
relays app↔hub frames, and refuses our own writes while the app is attached.
This proxy is also our capture tool (see `../research/README.md`).

## Discovery and session frames are ordinary frames

Upstream documents the UDP packets as special formats ("a 5-byte magic
probe", "device id `C2` + MAC"), but every one is a normal frame `[ref]`,
checked on the corpus (`record.TestHubInfoCorpus`):

| Frame | Family | Payload |
|---|---|---|
| NOTIFY_ME probe, broadcast to UDP 8102 | `C1` | none (`A5 5A 00 C1 C0`) |
| NOTIFY_ME reply, broadcast to UDP 8100 | `C2` | hub info (below) |
| CALL_ME, to UDP 8102 | `C3` | 6 bytes (see below), client IPv4, TCP port (BE16) |
| CONNECT_READY beacon, to UDP 8100 | `C4` | hub id (6), `00` |

## Hub info (banner `0x02`, discovery reply `C2`) `[ref]`

The TCP banner (reply to `REQ_BANNER 0x0001`) and the discovery reply carry
the same payload:

| Offset | Field |
|---|---|
| 0–5 | hub id: the MAC on X2; MAC[0:5] + `4B` (X1) / `45` (X1S) |
| 6 | marker: `64` in discovery replies, `00` or `64` in banners |
| 7 | model: `01` X1, `02` X1S, `03` X2 |
| 8–11 | production batch, BCD date (`20 22 11 20` = 2022-11-20) |
| 12 | hub firmware (= mDNS `AVER`): X1 17, X1S 5, X2 8 observed |
| 13–14 | flags, vary by model and revision; not to be relied on |
| 15.. | hub name (GB2312 above ASCII, up to 30 bytes) |

Our X2 `[X2✓]`: discovery reply and banner as above; banner opcode
`0x1502`, marker `00`, flags `00 00`, firmware 8, id = MAC. CALL_ME and the
connect-back to a port in 8200+ work. After the update to firmware 9
(2026-10-30) the banner, the TXT `AVER=9` and a full dump are unchanged
apart from the version.

### CALL_ME from the Android app `[X2✓]`

Seen through `sbx proxy` (2026-10-30):

```
A5 5A 0C C3 | 80 F1 B2 96 2C 08 | C0 A8 01 34 | 1F 41 | B8
            | the hub's id      | 192.168.1.52 (BE) | port 8001
```

The app puts the hub's id in the first six bytes (we send zeros; the hub
takes either), writes its IP **big-endian** (upstream saw little-endian
`[ref]`; the hub uses the source address anyway) and listens on TCP 8001.

### A busy hub `[X2✓]`

While a client (the app) is connected, the hub closes UDP 8102: a CALL_ME
gets an ICMP port-unreachable, which a connected UDP socket reports as
"connection refused". `transport` reports it as `ErrRefused`; the same
happens for an address that is not a hub at all.

### Reboots and liveness `[X2✓]`

At the end of a firmware update the hub reboots without closing its TCP
connection: no FIN or RST reaches the client, so a blocked read waits until
TCP keep-alive gives up. `transport` sets short keep-alive probes on hub
connections (`transport.KeepAlive`: 5 s idle, 3 probes 5 s apart). The
app, meanwhile, goes back to discovery (`NOTIFY_ME`) and waits for the hub
to come back, presumably for its new firmware number. A proxy must
therefore stop answering for the hub while it is away and re-read the
banner once it is back, which `proxy.Run` does.

## TODO

- [ ] Does the hub send any keep-alive of its own, and does it drop idle
      clients? (none seen in a 1.5 h idle proxy session)
- [ ] Port list (UDP/TCP) and firewall notes: hub UDP 8102 (CALL_ME,
      probes), discovery replies broadcast to UDP 8100, connect-back to the
      client's TCP port (upstream uses 8200–8263; whether the hub accepts
      any port is to be checked)
- [ ] Does the hub answer CALL_ME over UDP before connecting? (upstream says
      it echoes a CALL_ME; `transport` does not wait for it)
