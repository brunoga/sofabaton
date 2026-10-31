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

### Checked against a manufacturer's IR list `[X2✓]`

AWOL Vision publishes its Aetherion projectors' IR list (NEC, system code
`0x00CE`, one key value per button; `testdata/x2/aetherion-ir-list.json`).
Sofabaton built the "Aetherion Max" profile from it, and the app wrote all
26 of its commands as `P:NEC R:38400 D:0 S:206 F:<key value>`: the system
code's high byte is `D`, its low byte `S`, the key value `F`, in decimal
(`capture.TestX2Aetherion`). Hisense's `D:0 S:191` is its system code
`0x00BF` the same way. Power on, Power off, HDMI 1 to 3 and Home, sent by
the hub, all worked on the projector. Sofabaton left 9 of the list's 35 keys out of the
profile (Focus, Dashboard, LOW POWER, DP, Composite, Free TV, Favorite and
two key combinations).

So an NEC device's profile can be built from its published IR list alone.
We did it for one key: `sbx add-ir` wrote Focus (`F:217`, missing from
Sofabaton's profile) from the list, and the projector responded
([write-flows.md](write-flows.md#adding-one-command-to-a-device-x2)).

### Command codes are label ids `[X2✓]`

The 6-byte code of each command is the same across brands for the same
label: Power on `1713`, Power off `1718`, Home `07C7`, Netflix `0D36`, Ok
`002A`, Mute `006A` on both the Hisense and the Aetherion. Labels spelled
differently get different codes (`Navigate_up` `002E`, `Navigate up`
`4FAF`), so the code looks like an id in Sofabaton's dictionary of key
names, not part of the signal. Buttons and macros refer to commands by it.

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
