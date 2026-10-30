# Records

Reference: upstream `docs/protocol/data-structures.md`, `inputs.md`.

## Device / activity body (families 0x07, 0x37, 0x08) `[ref]`

Write wrapper: `01 | page(2, BE)=1 | body`. Catalog rows (families `0x0B`,
`0x3B`) carry the same body behind `row_index | 00 01`, with `body[0]` = the
burst's row count instead of the `01` page marker. `[ref]`, checked against
the upstream captures (`record.TestCatalogRow*`).

**Stored checksums are stale.** The body's last byte is
`sum(body[:-1]) & 0xFF` in writes (the app's X1 Bose create and update
verify), but catalog rows mostly fail it, real captures included: the hub
changes stored bytes (body[0], tail flag, …) without updating it. Decoders
must not require it; writers always recompute it. `[ref]`

`record.Entity` reproduces the app's X1 Bose create and update writes byte
for byte, both decoded and built from scratch.

| Offset | Field |
|---|---|
| 0 | body marker `01` |
| 1–2 | total pages (BE) |
| 3 | record kind |
| 4 | entity id (`FF` = create new) |
| 5 | icon |
| 6 | sort order |
| 7 | code/source type (device class byte) |
| 8 | device/activity type (category) |
| 9–24 | 16-byte code id (IR database / BT profile — opaque, see R1) |
| 25 | hide flag |
| 26 | input flag |
| 27 | channel / variant selector |
| 28 | power state (hub-maintained) |
| 29.. | name slot, brand slot, tail slot (30 B X1, 60 B X1S/X2) |
| last | `sum(body[:-1]) & 0xFF` |

### Device tail slot `[ref]`

| Tail offset | Field |
|---|---|
| 0–5 | `FC 55` + IPv4 (network classes) |
| 6–8 | `FC` + poll time (BE16); absent when byte 6 isn't `FC` |
| 9–13 | `FC` input_mode, power_mode, power_style, share_mode |
| 14–15 | `FC` flag (hub-maintained, changes after writes) |
| 16–17 | `FC` marker (0 in a create, 1 in the update that commits it; 1 on generic HTTP devices) |
| 18–21 | optional `FC` + 3 bytes; X2: IR emitter routing (last byte: bit0 hub, bit1 port 1, bit2 port 2, bits 3–7 remotes 1–5) |
| 22 | color, X2 palette 1–12 (60-byte tail only) |
| 23 | Bluetooth link mode, 0 classic / 2 BLE (60-byte tail only) |
| 24.. | unknown, preserved |

**Activity tails differ** (`fc 00 fc 01 …` on X1, `fc 00 00 …` on X1S);
upstream reads a "needs confirm" flag from the last `fc XX fc YY` pair.
Kept raw until decoded. `[ref]`

**Preserve unknown bytes on rewrite.** Upstream decodes the record into
fields and rebuilds it, zeroing tail bytes 24+ and any slot bytes after a
label's terminator; `record.Entity` keeps the decoded bytes and rewrites
only fields that changed.

## TODO (each section: layout, paging, encoding, captures)

- [ ] Catalog rows (devices `0x..0B`, activities `0x..3B`)
- [ ] Command records (`REQ_COMMANDS`, family `0x5D`) and command writes (`0x0E`)
- [ ] Keymap / favorites rows (family `0x3D`) and binding writes (`0x3E`)
- [ ] Activity membership (family `0x6D`)
- [ ] Macros (family `0x13` read, `0x12` write)
- [ ] Inputs page (family `0x46`/`0x47`)
- [ ] IR blob dump/save/replay pages (`0x0D`/`0x0E`/`0x0F`)
- [ ] Button code table
