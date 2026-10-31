# Hub firmware update

Observed once `[X2✓]`: the Android app updating our X2 from firmware 8 to 9
through `sbx proxy` (2026-10-30, 217 chunks in about 60 s). The capture is
`testdata/x2/app-firmware-update.jsonl`, with the image bytes removed, and
`capture.TestX2FirmwareUpdate` checks the flow below against it. Field meanings
marked `[guess]` rest on that single capture. We do not implement updates,
and the image itself is not kept in the repository (it is the vendor's).

## Flow

```
app → hub  29 FirmwareStart   once
hub → app  2B FirmwareAck     n = 0
app → hub  2A FirmwareChunk   n = 1 … N, one at a time
hub → app  2B FirmwareAck     after each chunk but the last, echoing n
hub → app  2C FirmwareDone    in place of the last chunk's ack
           (the hub reboots, without closing the TCP connection)
app        back to discovery until the hub answers again
```

It is stop-and-wait: the app sends the next chunk only after the previous
ack (about 0.3 s per round trip on our network).

All four payloads start with the same six bytes, `00 00 01 00 00 00`
`[guess]`: a target selector (the hub itself, as opposed to a remote).

## `29` FirmwareStart (26 bytes)

```
00 00 01 00 00 00 | 00 00 C7 64 | 46 1B B4 EF 25 D5 1A 1B 35 F7 21 9A ED 5B 30 C0
selector          | size? BE32  | digest? 16 bytes
```

`[guess]` 0xC764 = 51 044 bytes; the chunks carry 51 048, the last four of
which may be a trailer. The 16 bytes are not the MD5 of the image or of its
first 51 044 bytes; possibly a digest of the decrypted image.

## `2A` FirmwareChunk (up to 245 bytes)

```
00 00 01 00 00 00 | 01 | 00 01 | 236 bytes of image
selector          | ?  | n BE16
```

`n` counts from 1. Every chunk but the last carries 236 bytes. The image
looks encrypted or compressed (no structure).

## `2B` FirmwareAck (8 bytes) and `2C` FirmwareDone (6 bytes)

```
2B: 00 00 01 00 00 00 | 00 D8     n BE16 of the acknowledged chunk (0: the start)
2C: 00 00 01 00 00 00
```

## Open questions

- How the app learns that an update is available (an app-to-cloud
  exchange, not seen on the hub link) and whether remotes update the same
  way with another selector.
- What the hub does with a bad chunk or digest (no negative ack seen).
