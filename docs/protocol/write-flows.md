# Write flows

Reference: upstream `docs/protocol/write-flows.md`, `ack-handling.md`,
`live-hub-testing.md`.

## Ack model `[ref]`

Writes are strictly serial: send, wait for ack, then next step. Generic status
ack `0x0103`: `payload[0] == 0` accepted, non-zero rejected. Some families ack
with their own opcode and echo a byte (see table). No reply → timeout.

| Family | Purpose | Ack |
|---|---|---|
| `0x07` | create device | `0x0107`, `p[0]` = new device id |
| `0x37` | create activity | `0x0137` (seen on X1) `[ref:X1/X1S]` |
| `0x08` | update/finalize device | `0x0103` |
| `0x0E` | write a command/code record (paged) | `0x0103` |
| `0x3E` | write a button binding | `0x013E`, `p[0]` = button id |
| `0x0241` | device idle/power behaviour | `0x0103` |
| `0x12` | write a macro | `0x0112` (`p[0]` usually the key id; not always) |
| `0x46` | inputs page | `0x0103` |
| `0x61` | device key-sort page | `0x0103` |
| `0x0064` / X2 `0x0364` | remote sync | `0x0103` |

## Device create (non-wifi) `[ref:X1/X1S]`, partly X2 `[ref]`

1. `0x07` header (id `FF`) → capture assigned id.
2. `0x0E` command writes.
3. `0x3E` bindings.
4. optional `0x0241`, `0x12` macros, `0x46` inputs.
5. `0x08` finalize with the real id.
6. remote sync.

### Observed on the X2 `[X2✓]`

The Android app adding a Hisense TV from the cloud catalog, with the hub as
the emitter (2026-10-31, `testdata/x2/app-add-ir-device.jsonl`, checked by
`capture.TestX2AddIRDevice`). It took 33 s, all of it on the hub link. The
cloud only supplied the content; nothing the hub needs comes from anywhere
else:

| # | Frame | Count | Ack |
|---|---|---|---|
| 1 | `07` CreateDevice, id `FF`, name, catalog code id, emitter routing `00 00 07` | 1 | `07` with the new id (`04`) |
| 2 | `0E` WriteCommand, one per command (below) | 61 | `03` accepted, each |
| 3 | `3E` WriteButton: button id → the command's 6-byte code | 36 | `3E` echoing the button |
| 4 | `41` SetIdleBehavior `04 01` | 1 | `03` |
| 5 | `12` WriteMacro: `POWER_ON` and `POWER_OFF`, each one step | 2 | `12` echoing the key |
| 6 | `08` UpdateDevice: the create's record with the real id and the tail's commit marker set to 1, nothing else changed | 1 | `03` |

No remote-sync frame follows (as upstream saw `[ref]`): the hub updates
the remote itself. Deleting the device (`09` with its id) was acknowledged
only after 13.5 s.

#### `0E` WriteCommand body `[X2✓]`

Each write is one page, `[seq, 00 01]` with `seq` counting from 1, then:

```
3D 00 01          command count (61), pages of this record
04 00 0D          device id, command id (00: the hub assigns), type IR
00 00 00 00 17 18 code: the catalog's function id; buttons and macros refer to it
50 00 6F 00 ..    label, 60 bytes UTF-16BE ("Power off")
00 1D             descriptor length
00 00 11 00 94 70 magic (upstream's name; 0x9470 is 38 000)
"P:NEC R:38400 D:4 S:251 F:114"
00 00 00 00       padding
70                sum8 of the body from the count byte
```

The signal is the **descriptor text**: protocol and parameters, rendered
into IR by the hub. See [ir-codes.md](ir-codes.md).

## Adding one command to a device `[X2✓]`

A burst of one: `seq` 1, count 1, the record with the next free command
id (the app's creates send id 0 and let the hub number them; upstream
gives an explicit id here), then the data. `hub.Client.AddCommand` and
`sbx add-ir` do this; `record.TestCommandWriteMatchesTheApp` rebuilds all
87 of the app's captured command writes byte for byte with the same
encoder.

First write of ours to our X2 (2026-10-31): "Focus" on the Aetherion Max,
`P:NEC R:38400 D:0 S:206 F:217` from AWOL Vision's IR list (a key
Sofabaton's profile left out). Status ack `00`; the hub lists it as
command 27, keeps code `000000000000` as written and gives it sort `FF`
(no position on the remote's device page; upstream follows up with a
`61` sort-table write, not done yet). Sent from the app, it opened the
projector's focus screen.

## Renaming a command `[X2✓]`

The same write over an existing id, with the command's current code and
data and a new label. On our X2 (2026-10-31), "Ok" → "OK" kept id 24, code
`00000000002A` and the remote's OK binding; then `sbx apply` renamed 17
more of the Aetherion's commands and added its 8 remaining keys
([profiles.md](../profiles.md)).

The hub's reply to a command data request (`0C [device, id]`, answered in
`0D`) carries the same body as a write, but its last byte is not the sum8
(upstream calls it a replay tail); writers recompute it.

## Deleting one command `[X2✓]`

`10 [device, command id]`, status ack after a hub-side sweep (upstream
timed up to 12 s on big X1 configurations; ours took under 2.5 s with the
connection); the hub drops bindings that used the command `[ref]`. The
display-sort table keeps the deleted id until rewritten `[ref]`.
`sbx remove-ir` and `hub.Client.DeleteCommand`: on our X2 (2026-10-31)
Focus was deleted, the hub stopped listing it, and adding it again gave it
the same id, 27.

## Activity create `[ref:X1/X1S]`

`0x37` header → `0x3E` bindings → `0x12` macros → remote sync.

## TODO

- [ ] Delete device/activity, membership, favorites, ordering (`0x11`, `0x51`, `0x61`, `0x65`)
- [ ] Paging rules per family (`0x0E` uses `[command_seq, page_be]`)
- [ ] X2 remote-sync targeting and "busy updating remote" status `0x11`
