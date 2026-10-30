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
| CALL_ME, to UDP 8102 | `C3` | 6 zero bytes, client IPv4, TCP port (BE16) |
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
connect-back to a port in 8200+ work.

## TODO

- [ ] Keepalive behaviour and idle timeouts
- [ ] Port list (UDP/TCP) and firewall notes: hub UDP 8102 (CALL_ME,
      probes), discovery replies broadcast to UDP 8100, connect-back to the
      client's TCP port (upstream uses 8200–8263; whether the hub accepts
      any port is to be checked)
- [ ] Does the hub answer CALL_ME over UDP before connecting? (upstream says
      it echoes a CALL_ME; `transport` does not wait for it)
