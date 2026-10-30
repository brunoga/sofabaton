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

## Labels in record slots

Name and brand slots are zero-padded; text ends at the first NUL (X1) or
zero code unit (X1S/X2). Differences from upstream, both deliberate and
pinned by `record.TestOracleEntity*`:

- **Cut surrogate pairs.** Upstream's device-record writer cuts the UTF-16
  bytes at the slot width and can leave a lone high surrogate (e.g. 29
  letters + 📺 in a 60-byte slot ends in `D8 3D`). Its own label writer for
  commands and macros avoids that. We never write half a pair, and on
  decoding drop unpaired surrogates (as upstream does), so such labels read
  as their intact prefix.
- **Whitespace.** Upstream trims decoded labels; we keep the exact text so it
  round-trips, and leave trimming to the UI.

## Paged replies

Multi-page replies share one structure `[ref]`, checked against every
complete command-list frame in the upstream corpus (47 frames, 14 bursts):

```
page payload = marker(01) | page (BE16) | chunk
body         = chunk 1 | chunk 2 | …        (wire.Assembler)
body[0]      = marker 01
body[1:3]    = page count (BE16)
body[3..]    = family-specific content
```

Upstream names "header", "body", "final" and "single-command" pages and a
dozen full opcodes (`0xD95D`, `0xD55D`, `0x495D`, `0x4D5D`, `0xF75D`, …);
they are all this structure at different page sizes. A targeted
single-command reply is a 1-page body with a count of 1.

## Command list (`REQ_COMMANDS` reply, family `0x5D`) `[ref]`

Body content: `count | records…`, fixed-width records, `count × stride`
bytes exactly. In every capture each page holds whole records (6 on X1,
3 on X1S/X2), but decoding works on the joined body regardless.

| Record offset | Field |
|---|---|
| 0 | device id |
| 1 | command id |
| 2 | command type (a class byte: `0D` IR, `1C` HTTP, `20` MQTT, `0A` Roku, `1A` Hue, …) |
| 3–8 | 6-byte code (opaque) |
| 9.. | label slot: 30 B ASCII (X1) / 60 B UTF-16BE (X1S/X2) |
| last | sort byte: equals the command id on X1S/X2 captures, `FF` on X1 ones |

Stride 40 (X1) / 70 (X1S/X2). UTF-16 labels can contain `FF` bytes, so
records are never split on `FF`. `record.ParseCommandList` matches
upstream's decoding of all 8 complete corpus bursts and re-encodes each
record to its bytes (`TestOracleCommandBursts`).

## TODO (each section: layout, paging, encoding, captures)

- [ ] Command writes (`0x0E`)
- [ ] Keymap / favorites rows (family `0x3D`) and binding writes (`0x3E`)
- [ ] Activity membership (family `0x6D`)
- [ ] Macros (family `0x13` read, `0x12` write)
- [ ] Inputs page (family `0x46`/`0x47`)
- [ ] IR blob dump/save/replay pages (`0x0D`/`0x0E`/`0x0F`)
- [ ] Button code table
