# Connection and discovery

Reference: upstream `docs/protocol/connection-flow.md`, `hub-versions.md`.

## Outline

1. Discovery `[ref]`: mDNS (X2 observed advertising `_sofabaton_hub._udp.local.`;
   TXT carries `HVER`: 1=X1, 2=X1S, 3=X2) or UDP broadcast `NOTIFY_ME`.
2. Client sends `CALL_ME` (UDP, opcode `0x0CC3`, 12-byte payload: 6 reserved
   bytes, a local IPv4, a big-endian listen port, e.g. 8200) `[ref]`.
   The hub appears to use the UDP source address, not the IP field `[ref]`.
3. **The hub connects back** over TCP to the client's listen port `[ref]`.
4. Banner/identity exchange, then catalog requests `[ref]`.

## Single client

The hub serves one client at a time `[ref]`. To keep the Android app usable,
we run a proxy that advertises itself as the hub (mirrored mDNS name/TXT),
relays app↔hub frames, and refuses our own writes while the app is attached.
This proxy is also our capture tool (see `../research/README.md`).

## TODO

- [ ] Banner layout (model code, batch date, firmware) — confirm on X2
- [ ] NOTIFY_ME reply layout
- [ ] Keepalive behaviour and idle timeouts
- [ ] Port list (UDP/TCP) and firewall notes
