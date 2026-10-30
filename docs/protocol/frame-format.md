# Frame format

Status: layout and checksum verified against the 125 complete upstream
captures (`[ref]` captures, our own X2 captures pending).
Reference: upstream `docs/protocol/frame-format.md`.

## Layout

Every TCP message, and the UDP `CALL_ME`, is one frame:

```
A5 5A | op_hi op_lo | payload (op_hi bytes) | sum8
```

- `A5 5A` — sync.
- Opcode — 16-bit big-endian. **`op_hi` is the payload length**; `op_lo` is
  the *family* (related opcodes share it, e.g. paged replies of one request).
  So the same logical message gets a different full opcode per page size,
  and a message is identified by its family and direction, not by the full
  opcode. Upstream names many opcodes that are just page sizes of one family
  (`0xD55D`, `0xF35D`, `0x4D5D`… are all family `0x5D`).
- Some families mean different things per direction (`0x05`: leave learn
  mode A→H, learn window closed H→A; `0x01`: banner request A→H, save ack
  H→A). `[ref]`
- Checksum — `sum(all preceding bytes, including sync) & 0xFF`.

Frame length is therefore `5 + op_hi` (max 260 bytes).

Example, request activity list (`0x003A`, empty payload):
`A5 5A 00 3A 39` — `0xA5+0x5A+0x00+0x3A = 0x139`, so the checksum is `0x39`.

> Upstream's frame-format doc gives `79` for this example, which is wrong.
> The rule itself holds: all 125 complete upstream captures verify with it
> (`wire.TestUpstreamCaptures`).

## Deframing

- Fast path: when aligned, read 4 bytes, then `op_hi + 1` more.
- Recovery: on a checksum mismatch, move one byte forward and scan for the
  next `A5 5A`. Never trust a frame whose checksum fails. Near the end of a
  stream, a corrupted length byte can announce more bytes than are left;
  look for a valid frame in what is left before reporting a cut frame.
  (`wire.Decoder` does both; `wire.Decoder.Skipped` counts the bytes lost.)
- The Go deframer must be fuzzed: arbitrary input must never panic or
  allocate unboundedly.

## Multi-frame replies

Many requests answer with a *burst* of frames. Page boundaries do not align
with record boundaries (records can be split across pages; UTF-16BE labels
can contain `0xFF`), so: collect the burst, concatenate page bodies in order,
then decode records from the concatenation.

## Byte order and text

- Integers: big-endian unless noted.
- Text is per record type: X1S/X2 labels are mostly UTF-16BE in fixed-width
  slots; X1 mostly ASCII/UTF-8; mDNS TXT and HTTP callbacks are UTF-8.
  Encoding is chosen by record type, never globally.

## Direction notation

`A→H` app/client to hub, `H→A` hub to app/client.

## Upstream constants that break the length rule

A few upstream opcode constants cannot be real opcodes under the rule above,
for example `OP_CREATE_DEVICE_HEAD = 0x07D5` (7-byte payload, family `0xD5`),
which looks like `0xD507` with its bytes swapped; device creates are family
`0x07` with long payloads (`0xD507` on X1S/X2, `0x7B07` on X1). `[guess]`
Our code never uses full opcodes as identities, so these do not matter for
decoding; they are noted in case a capture shows otherwise.
