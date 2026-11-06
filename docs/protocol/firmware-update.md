# Hub firmware update

Observed once `[X2✓]`: the Android app updating our X2 from firmware 8 to 9
through `sbx proxy` (2026-10-30, 217 chunks in about 60 s). The capture is
`testdata/x2/app-firmware-update.jsonl`, with the image bytes removed, and
`capture.TestX2FirmwareUpdate` checks the flow below against it. Field meanings
marked `[guess]` rest on that single capture. The image itself is not kept in
the repository (it is the vendor's).

`hub.Client.UpdateFirmware` sends an image this way, and `sbx firmware` gets
it as the app does (below); our start frame for the 8 to 9 image, downloaded
again on 2026-11-06, is the app's byte for byte. No newer hub firmware has
been out since, so we have not sent one to a hub yet.

## Where the image comes from `[X2✓]`

The app asks Sofabaton's server (`app/checkFirmwareUpgrade`, the catalog's
encrypted API) with the hub's product name (`x1`, `x1s`, `x2`), firmware
(`software_code`), hardware version (`hardware_code`: the mDNS `HVER`, 1, 2
and 3 by model) and production batch (`production_time`, the mDNS `NO`,
`20221120`). The answer names the newer firmware and the URL of its image
on the vendor's S3 store; code 1 means none is newer. Asked for our X2 on 9,
it had none; asked as if on 8, it offered 9, whose image is exactly what the
app sent. The remote (`remote2-manager`, then `remote2_release`) and the
repeater (`repeater_release`) are asked the same way; the X2 remote's update
goes from the phone to the remote over Bluetooth, not through the hub.

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

0xC764 = 51 044 bytes, the image's size, and the 16 bytes its MD5 `[X2✓]`
(checked against the image downloaded again: an earlier count of 51 048 bytes
in the chunks, and an MD5 that did not match, were of bytes of the frames,
not the image's). 217 chunks of 236 bytes, the last of 68, are the image.

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

- What the hub does with a bad chunk or digest (no negative ack seen).
