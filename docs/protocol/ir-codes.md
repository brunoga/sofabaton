# IR codes

Reference: upstream `docs/protocol/data-structures.md` (blob sections),
`IrScrutinizer/x2-protocols.md`, `docs/command_payloads.md`.

## Forms `[ref]`

- **Raw timing blobs** — carrier + mark/space timings; all hubs; what
  hub-learning produces. Convertible to/from Pronto hex.
- **X2 descriptive** — ASCII `P:<protocol> R:<carrier> D:.. F:.. ...`, the hub
  synthesizes the signal. Normally shipped by Sofabaton's cloud catalog.
  Upstream photon-verified many tokens (NECx, Sony12/20, RC5, RC6, Samsung36,
  Panasonic, Sharp family, DirecTV1, Bose, …), others partial.

## Why this matters

Generating either form locally from open IR databases removes the dependency
on the cloud catalog for "known" devices (limitation L3, research R3).

## TODO

- [ ] Raw blob byte layout and trailing-byte normalization
- [ ] Save pages (family `0x0E`) incl. trailing checksum
- [ ] Replay (family `0x0F`) — note: plays ignore X2 emitter routing `[ref]`
- [ ] Token/field table for `P:` with our own verification status
