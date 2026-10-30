# Frame format

Status: `[ref]` throughout, pending confirmation from our first X2 captures.
Reference: upstream `docs/protocol/frame-format.md`.

## Layout

Every TCP message, and the UDP `CALL_ME`, is one frame:

```
A5 5A | op_hi op_lo | payload (op_hi bytes) | sum8
```

- `A5 5A` — sync.
- Opcode — 16-bit big-endian. **`op_hi` is the payload length**; `op_lo` is
  the *family* (related opcodes share it, e.g. paged replies of one request).
  So the same logical message gets a different full opcode per page size.
- Checksum — `sum(all preceding bytes, including sync) & 0xFF`.

Frame length is therefore `5 + op_hi` (max 260 bytes).

Example, request activity list (`0x003A`, empty payload):
`A5 5A 00 3A 79`.

## Deframing

- Fast path: when aligned, read 4 bytes, then `op_hi + 1` more.
- Recovery: on a checksum mismatch, scan forward for the next `A5 5A` and
  retry. Never trust a frame whose checksum fails.
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
