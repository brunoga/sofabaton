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
gives an explicit id here), then the data. `hub.Client.WriteCommand` and
`sbx add-ir` do this; `record.TestCommandWriteMatchesTheApp` rebuilds all
87 of the app's captured command writes byte for byte with the same
encoder.

First write of ours to our X2 (2026-10-31): "Focus" on the Aetherion Max,
`P:NEC R:38400 D:0 S:206 F:217` from AWOL Vision's IR list (a key
Sofabaton's profile left out). Status ack `00`; the hub lists it as
command 27, keeps code `000000000000` as written and gives it sort `FF`
(every command record on our X2 has sort `FF`: the remote's order is a
separate table, below). Sent from the app, it opened the projector's
focus screen.

## Renaming a command `[X2✓]`

The same write over an existing id, with the command's current code and
data and a new label. On our X2 (2026-10-31), "Ok" → "OK" kept id 24, code
`00000000002A` and the remote's OK binding; then `sbx apply` renamed 17
more of the Aetherion's commands and added its 8 remaining keys
([profiles.md](../profiles.md)).

The hub's reply to a command data request (`0C [device, id]`, answered in
`0D`) carries the same body as a write, but its last byte is not the sum8
(upstream calls it a replay tail); writers recompute it.

## Running commands now `[X2✓]`

`5F` carries a macro body that the hub runs at once and does not store:
the entity (the device), key `00`, the steps, an empty label. The app uses
it to test a power command while setting a device up (`record.TestRunMacroMatchesTheApp`
rebuilds both captured frames); each step names its command by device,
id and code, delay `FF` for a single step. Status ack `00`.
`hub.Client.Run` and `sbx send`: on our X2 (2026-10-31) Focus and Power Off
sent this way worked on the projector.

## Deleting one command `[X2✓]`

`10 [device, command id]`, status ack after a hub-side sweep (upstream
timed up to 12 s on big X1 configurations; ours took under 2.5 s with the
connection); the hub drops bindings that used the command `[ref]`. The
display-sort table keeps the deleted id until rewritten `[ref]`.
`sbx remove-ir` and `hub.Client.DeleteCommand`: on our X2 (2026-10-31)
Focus was deleted, the hub stopped listing it, and adding it again gave it
the same id, 27.

## Key bindings `[X2✓]`

`3E`, one key per write: `01 0001 01 0001`, the 18-byte row (entity,
button, short press `device code6 command`, long press the same), sum8 of
the body from its fourth byte. The hub acks with `3E [button, ?]`; the
second byte is the button plus `3E` in every ack we saw. A write replaces
the key's row.

The app names the command by **code alone** (command id `00`); the hub
fills in the id, and the keymap reads back (`3C [entity, FF]` → `3D`) with
both. `capture.TestX2ButtonWrites` rebuilds all 50 of the app's key writes
from `record.Binding`. Commands we add have code `000000000000`, so we
name them by id and code `0` instead: on our X2 (2026-10-31) the hub
accepted that for all 7 keys of a new device, and the keymap reads back
with those ids (`sbx list buttons`).

Key ids are in `record/button.go`. Device keymaps written by the app
also carry `AB` (the power key, bound to the device's power toggle) and
`C2`/`C3`, bound to its power on and off commands. `C2`/`C3` are not keys
of the remote; what reads them is not known `[inferred]`.

## Command order `[X2✓]`

The order of a device's page on the remote is a table of
`(command id, position)` pairs, positions from 1, `FF` (or `00`) for none
`[ref]`. It is read with `62 [device]`, answered in `63` with
`01 pages16 device pairs...` (no checksum). It is written with `61`: the
same body plus a sum8, in pages behind `01 page16`, status ack. Upstream
saw an X1S refuse a table that places nothing (status `06`) `[ref]`.

On our X2 (2026-10-31), the three IR devices the app created had **no
table** (`62` answered status `07`). The Bluetooth device had one placing
none of its 25 commands. The remote presumably lists the commands of a
device without a table by id `[inferred]`. Our `61` write of the
Aetherion's 35 commands in the profile's order was accepted and read back
as written.

## Device create from scratch `[X2✓]`

The app's create flow (above), with our own records and no catalog. On
our X2 (2026-10-31), `sbx apply -create` made "ZZ Test" (8 Aetherion
codes) in 5.7 s:

1. `07` a new record (id `FF`): class IR, type 2 and icon 1 (the Hisense's),
   code id all zeros, `record.NewIRDeviceTail` (emitters hub + both ports,
   marker 0). Ack `07 05`.
2. `0E` each command as a burst of one, ids 1..8, code 0.
3. `3E` the keys, naming commands by id (above).
4. `41 [05, 01]`.
5. `12` `POWER_ON` (`C6`) and `POWER_OFF` (`C7`), one step each, by id.
6. `08` the record with id 5 and marker 1.

Every step was acked; the device lists, its keys and sequences read back
as written. `capture.TestX2CreateFrames` rebuilds the app's create,
idle-behavior, power-sequence and commit frames from records made from
scratch. What a zero code id or our type and icon change on the remote is
still to be seen.

### Inputs `[X2✓]`

When the user says the device switches inputs, the app writes, after the
commit:

1. `46` the inputs page, mode 1 (each input its own command), the entries
   `command code6 ordinal label`, status ack per page.
   `capture.TestX2InputsWrite` rebuilds the Aetherion's (HDMI 1-3, Home)
   byte for byte from `record.Inputs`.
2. `08` the record with the tail's input mode 1. (Its power setup before
   that also rewrote the record with power mode 1 and style 3.)
3. `65 [device, 01]`, the X2's per-device sync to the remotes `[ref]`.

Re-creating ZZ Test with HDMI 1 and 2 as inputs, our X2 accepted all of it
and the page reads back as written; `sbx remove-device` deleted the first
ZZ Test in 3.8 s.

## Remote sync `[X2✓]`

The remote keeps its own copy of each device. After our renames, adds and
order write on the Aetherion (no sync), our X2's remote still showed the
original catalog list: old names ("disnesy", "Power") in id order. The
freshly created ZZ Test, whose create ended with `65 [device, 01]`, was
current: its two power sequences first, as "Turn On" and "Turn Off", then
its 10 commands in order. `apply` now ends every change with that sync;
`sbx sync -device X` sends it alone.

## Activities `[ref:X1/X1S]`

Upstream's flows, validated on X1 and X1S, none yet on the X2; ours wait
for a capture of the X2 app.

- **Create:** family `0x37` with a device-shaped record (id `FF`), acked
  by `0x37` with the new id; no commit record. Then key bindings (`0x3E`,
  entity = the activity), macros (`0x12`) and a remote sync. The hub
  appends the activity to the end of the list.
- **Members:** each member is written with `0x4F [device, 00]` (the full
  list, in order, each status-acked). The activity's POWER_ON then gets
  the device's `C6` step (its POWER_ON sequence) and `C5` step (its input:
  the step's duration byte is the input's ordinal in the device's inputs
  page, 0 for none), POWER_OFF its `C7` step. The macros' label slots must
  be rewritten as read: a rebuilt slot is refused (status `0C`).
- **Removing a member:** no frame of its own. Rewriting POWER_ON without
  the device's steps makes the hub drop it from POWER_OFF, the members,
  the keys and the favorites.
- **Keys:** `0x3E` as for devices; a key that runs one of the activity's
  macros names the activity as its device and the macro's id as its
  command.
- **Sequences (user macros):** `0x12` at a free id (1-197, shared with
  favorites; `C6`/`C7` are the power sequences). The app writes id `00`
  and the hub answers with the id it gave. Steps are 10 bytes (device,
  command, code, duration, delay); a pause is all `FF` but its delay
  (seconds). The remote lists them with the favorites, in the order of
  the activity's `0x61` table. Delete: `0x10 [activity, id]`.
- **Delete:** `0x09 [activity]`.
- **Rename:** a rewrite of the activity's record.

Read on our X2 `[X2✓]`: activities' POWER_ON sequences are, per member,
a `C6` step and a 1-second pause, then per member a `C5` step and a pause
(the `C5` duration is the ordinal, 0 for members not switched); `C6`
steps carry duration 1 for IR devices and 0 for Bluetooth ones.

## TODO

- [ ] Delete activity, membership, favorites, device and activity ordering (`0x11`, `0x51`, `0x65`)
- [ ] Paging rules per family (`0x0E` uses `[command_seq, page_be]`)
- [ ] X2 remote-sync targeting and "busy updating remote" status `0x11`
