# Records

Reference: upstream `docs/protocol/data-structures.md`, `inputs.md`.

## Device / activity body (families 0x07, 0x37, 0x08) `[ref]`

Write wrapper: `01 | page(2, BE)=1 | body`.

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

Tail slot `[ref]`: IP marker + IPv4 (0–5), poll-time (6–8), input/power/share/
tail-marker region (9–17), vendor block (18–21; on X2 `FC` + IR routing:
bit0 built-in, bit1 port1, bit2 port2, bits3–7 remotes 1–5), 22 color (X2),
23 Bluetooth link mode (0 classic, 2 BLE).
**Preserve unknown bytes on rewrite.**

## TODO (each section: layout, paging, encoding, captures)

- [ ] Catalog rows (devices `0x..0B`, activities `0x..3B`)
- [ ] Command records (`REQ_COMMANDS`, family `0x5D`) and command writes (`0x0E`)
- [ ] Keymap / favorites rows (family `0x3D`) and binding writes (`0x3E`)
- [ ] Activity membership (family `0x6D`)
- [ ] Macros (family `0x13` read, `0x12` write)
- [ ] Inputs page (family `0x46`/`0x47`)
- [ ] IR blob dump/save/replay pages (`0x0D`/`0x0E`/`0x0F`)
- [ ] Button code table
