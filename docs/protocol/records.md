# Records

Reference: upstream `docs/protocol/data-structures.md`, `inputs.md`.

## Device / activity body (families 0x07, 0x37, 0x08) `[ref]`

Write wrapper: `01 | page(2, BE)=1 | body`. Catalog rows (families `0x0B`,
`0x3B`) carry the same body behind `row_index | 00 01`, with `body[0]` = the
burst's row count instead of the `01` page marker. `[ref]`, checked against
the upstream captures (`record.TestCatalogRow*`).

**Stored checksums are stale** `[X2✓]` (device records stale, activity
records and macros `00`, inputs pages valid on our X2). The body's last byte is
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

Activity membership rows (family `0x6D`, the reply to `REQ_ACTIVITY_MAP`)
are catalog rows too: one per member device, carrying that device's full
record (`record.TestMembershipRows`). `[ref]`

### Device tail slot `[ref]`

| Tail offset | Field |
|---|---|
| 0–5 | `FC 55` + IPv4 (network classes) |
| 6–8 | `FC` + poll time (BE16); absent when byte 6 isn't `FC` |
| 9–13 | `FC` input_mode, power_mode, power_style, share_mode |
| 14–15 | `FC` flag (hub-maintained, changes after writes) |
| 16–17 | `FC` marker (0 in a create, 1 in the update that commits it; 1 on generic HTTP devices) |
| 18–21 | optional `FC` + 3 bytes; X2: IR emitter routing (last byte: bit0 hub, bit1 port 1, bit2 port 2, bits 3–7 remotes 1–5) |
| 22 | color, X2 palette 1–12 (60-byte tail only); devices and activities `[X2✓]` |
| 23 | Bluetooth link mode (60-byte tail only): `01` on our X2's Bluetooth devices, made from catalog profiles of `mode` 1 (BLE), `FF` on IR devices `[X2✓]`; `00` for the catalog's mode 0 (classic Bluetooth) `[inferred]`; upstream's `2` for BLE is not what our X2 holds |
| 24.. | unknown, preserved |

**Compact tails** `[X2✓]`. The last update of upstream's create of a
generic HTTP device writes the tail without the address's six bytes:
`FC 00 01 FC 01 01 01 00 FC 01 FC 01`, poll time 1, input, power mode and
style 1, flag and marker 1, then zeros. Our X2 keeps it as written.
`record.ParseDeviceTail` takes a tail whose field markers sit at bytes 0,
3, 8 and 10 (and no `FC 55`) as compact (`DeviceTail.Compact`).

### HTTP command data (class `1C`) `[X2✓]`

| Offset | Field |
|---|---|
| 0–3 | IPv4 address |
| 4–5 | port (BE16) |
| 6–7 | length of the text (BE16) |
| 8.. | the request's text |

The text is `METHOD PATH HTTP/1.1`, `Host:IP:PORT`, the headers,
`Content-Type:…` if any, and with a body `Content-Length:N`, a blank
line and the body; a blank line ends it. Our X2 returned the requests we
wrote unchanged (`record.HTTPRequest`; upstream's `blob_decoders.py`).

### Web command data (type `21`, in `1C` devices) `[X2✓]`

The app writes the X2's generic HTTP commands in this newer form (and the
form above on the X1S), all big-endian:

| Field | Bytes |
|---|---|
| length of it all | 2 |
| URL length, URL (`http://` or `https://`, an address or a name) | 2 + n |
| method: 0 GET, 1 POST, 2 PUT | 1 |
| number of headers | 1 |
| each header: name length, name, value length, value | 1 + n + 2 + m |
| body | the rest |

Our X2 sent such requests with `User-Agent: ESP32 HTTP Client/1.0`, its
own `Host` and `Content-Length`, the headers in order, and
`Content-Type: application/x-www-form-urlencoded` when none was given;
over https it took a self-signed certificate (2026-11-06,
`record.WebRequest`). That it resolves names is the app's Govee and LIFX
commands' doing, not yet seen.

### Roku and Sonos command data (classes `0A`, `1B`) `[X2✓]`

A Roku's command is `u8 length | path` (`keypress/Home`, `launch/12`),
which the hub POSTs to port 8060 of the address in the device's record:
`POST /keypress/Home HTTP/1.1`, `Host:IP:8060`, a blank line. A Sonos's
is laid out as a Hue's, `u8 path length | u16 rest length | path |
rest`; the hub POSTs it to port 1400, the rest (a `SOAPACTION` line,
`Content-Length:N`, a blank line and the envelope) sent as it is, then a
`\r`. The app's Volume up and down are the paths
`MediaRenderer/VolumeUp/Control` and `VolumeDown/Control` with a volume
as the rest: the hub sent the last SetVolume it had sent, one up
(2026-11-06, devices pointed at a listener; `record.RokuRequest`,
`record.SonosRequest`). The devices are made as a Hue bridge is: type 16,
the address's last byte as the channel, the Hue tail.

The hub keeps the connection open and sends the next command on it
(HTTP/1.1 keep-alive): a server that waits for more before answering, or
closes after one request, loses the presses that follow. Against one that
answers at once and keeps the connection, every press arrived, each on the
same connection, one Up and OK 0.8 s apart included (2026-11-07, from the
remote, in an activity: power on and off, keys, a macro, a favorite).

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

## Paged replies: bursts of records

Every reply we have decoded is one structure `[ref]`, checked against the
corpus's command lists (47 frames), keymaps, macros and catalog rows:

```
page payload = record index | page (BE16) | chunk
record body  = chunk 1 | chunk 2 | …        (wire.Assembler)
body[0]      = number of records in the burst
body[1:3]    = pages of this record (BE16)
body[3..]    = the record
```

A reply is a burst of records 1..N, each a paged body (`wire.Burst`).
Catalog rows and macros are bursts of many single- or multi-page records;
command lists and keymaps are bursts of one record (marker and `body[0]`
both `01`) holding a count and fixed-width entries. The record bodies have
the layout of the corresponding write bodies, where `body[0]` is the page
marker `01` instead of the record count.

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

## Keymap (`REQ_BUTTONS` reply, family `0x3D`) and binding writes (`0x3E`) `[ref]`

The reply is a paged body (above) with content `count | rows…`, 18-byte rows
that do span pages (upstream's "rows split across pages"; 247-byte chunks
are not a multiple of 18). Each row is a stored binding, with the same
bytes as the body of a binding write:

| Row offset | Field |
|---|---|
| 0 | entity (activity, or device for device keymaps) |
| 1 | button: a favorite slot (low ids) or a hard key (`0x97`–`0xC7`) |
| 2 | short press: device |
| 3–8 | short press: command code (6 bytes, as in the command record) |
| 9 | short press: command id |
| 10 | long press: device (0 = none) |
| 11–16 | long press: command code |
| 17 | long press: command id |

Upstream calls bytes 9/17 a "button id" in its write builder and the
command id in its read docs; the rows in the corpus match the command
ids (favorite 1 → device 3 command 3). Upstream warns that hard-key rows do
not always name the real target (macro-bound keys, role pages); to verify
against our X2.

Binding write payload: `01 00 01 | 01 00 01 | row (18) | sum8(body)`, 25
bytes, acked by family `0x3E` echoing the button. `record.Binding`
matches upstream's builder on 100 generated bindings.

Hard keys `0x97`–`0xA9` (C, B, A, Exit, DVR, Play, Guide, numpad) exist on
the X2 remote only. Power on/off are `0xC6`/`0xC7`.

## Macros (`REQ_MACROS` reply, family `0x13`; writes `0x12`) `[ref]`

One record per macro of the entity, each possibly several pages (the X1S
corpus has a 20-step `POWER_ON` over two pages). Record body = write body:

| Body offset | Field |
|---|---|
| 0 | record count (read) / `01` (write) |
| 1–2 | pages of this record |
| 3 | entity (activity, or device) |
| 4 | macro id: the key it is bound to; `C6` power on, `C7` power off |
| 5 | step count |
| 6.. | steps, 10 bytes: device, command (`FF` = pause), code (6), duration, delay |
| then | label slot, 30 B ASCII (X1) / 60 B UTF-16BE (X1S/X2) |
| last | `sum8` of the body before it |

Upstream calls the read-side fields "fragment index", "total fragments",
"activity" and "macro id" at `payload[0]`, `[3]`, `[6]`, `[7]`: the same
bytes seen through the wrapper.

- **The label slot holds hub metadata** after the label (X1 `37 37 00 00 35
  35`, X1S `d4 d4` at slot bytes 54–55), and the hub rejects saves that
  change it (status `0x0C`). `record.Macro` keeps the slot verbatim while
  the label is unchanged.
- The hub's power labels can have other bytes before `POWER_ON`/`POWER_OFF`;
  they are found anywhere in the slot.
- Writes longer than a page (19+ steps on X1S/X2, 22+ on X1) are paged;
  every page but the last is acked by a status ack, the last by family
  `0x12`. A save that removes a device's last power reference makes the hub
  remove the device from the activity, and that ack's first byte is not the
  macro id.
- The X1S rewrites the duration of power-reference steps (command `C5`–`C7`)
  from the device's power setup: not round-trippable there.

`record.Macro` matches upstream's decoding of the 4 corpus bursts and its
writer on 24 generated macros (0–40 steps).

## Inputs page (writes `0x46`; replies `0x47` to `0x0148 [device]`) `[ref]`

A device's input list, four input control keys and ten favorite rows, one
paged record. The hub's reply has the write's layout; upstream calls the
request `REQ_ACTIVITY_INPUTS` though it takes a device id. A device with
no inputs page answers with a non-zero status ack.

| Body offset | Field |
|---|---|
| 0–2 | `01`, pages (BE16) |
| 3 | device |
| 4 | mode: `00` no inputs, `01` direct inputs, `02` no input switching |
| 5 | entry count |
| 6, 7 | start / restart position (0 observed) |
| 8.. | entries: X1 27 B (command, code (6), 20 B ASCII label); X1S/X2 48 B (command, code (6), ordinal, 40 B UTF-16BE label) |
| last 108 | 4 control-key rows × 9 B (list, up, down, confirm), 10 favorite rows × 7 B, a state byte, `sum8` |

The control-key and favorite rows are opaque so far. Upstream finds the
trailer from the end, allowing slack bytes after the entries; `record.Inputs`
does too and keeps them. No inputs page appears in the capture corpus: the
layout rests on upstream's builder and parser (21 generated pages, 0–12
entries, all matching) until our X2 captures arrive.

## TODO (each section: layout, paging, encoding, captures)

- [ ] Command writes (`0x0E`)
- [ ] IR blob dump/save/replay pages (`0x0D`/`0x0E`/`0x0F`)
- [ ] Button code table
