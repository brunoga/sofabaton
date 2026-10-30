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

Upstream observed the X2 app doing `0xD507` create then `0xD508` update, and
**no** remote-sync frame after create/rename (the hub syncs the remote itself)
`[ref]`. To confirm with our own capture.

## Activity create `[ref:X1/X1S]`

`0x37` header → `0x3E` bindings → `0x12` macros → remote sync.

## TODO

- [ ] Delete device/activity, membership, favorites, ordering (`0x11`, `0x51`, `0x61`, `0x65`)
- [ ] Paging rules per family (`0x0E` uses `[command_seq, page_be]`)
- [ ] X2 remote-sync targeting and "busy updating remote" status `0x11`
