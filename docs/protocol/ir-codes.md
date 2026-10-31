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

## What our X2 receives `[X2✓]`

Adding a catalog device in the app writes every command with a
descriptor such as `P:NEC R:38400 D:4 S:251 F:114` (protocol NEC, carrier
field 38400, device 4, subdevice 251, function 114) inside the command
record itself (layout in [write-flows.md](write-flows.md#0e-writecommand-body-x2)).
Hisense's 61 commands used two NEC address pairs (`D:0 S:191` and
`D:4 S:251`) and varied only `F`. The hub turns the text into the signal,
so the cloud catalog is a database of (protocol, parameters) per button,
the same kind of data open IR databases publish (device / subdevice /
function per protocol, e.g. irdb).

Open: which protocol names the X2 renders (upstream photon-verified many),
whether the catalog code id and the per-command codes must be real catalog
ids, and what a learned (raw) code looks like in the same write.

## Why this matters

Generating either form locally from open IR databases removes the dependency
on the cloud catalog for "known" devices (limitation L3, research R3).

## TODO

- [ ] Raw blob byte layout and trailing-byte normalization
- [ ] Save pages (family `0x0E`) incl. trailing checksum
- [ ] Replay (family `0x0F`) — note: plays ignore X2 emitter routing `[ref]`
- [ ] Token/field table for `P:` with our own verification status
