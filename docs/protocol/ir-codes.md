# IR codes

Reference: upstream `docs/protocol/data-structures.md` (blob sections),
`IrScrutinizer/x2-protocols.md`, `docs/command_payloads.md`.

## Forms `[ref]`

- **Raw timing blobs** — carrier + mark/space timings; all hubs; what
  hub-learning produces. Convertible to/from Pronto hex.
- **X2 descriptive** — ASCII `P:<protocol> R:<carrier> D:.. F:.. ...`, the hub
  synthesizes the signal. Normally shipped by Sofabaton's cloud catalog.
  Upstream photon-verified many tokens (NECx, Sony12/20, RC5, RC6, Samsung36,
  Panasonic, Sharp family, DirecTV1, Bose, …), others partial.

## What our X2 receives `[X2✓]`

Adding a catalog device in the app writes every command with a
descriptor such as `P:NEC R:38400 D:4 S:251 F:114` (protocol NEC, carrier
field 38400, device 4, subdevice 251, function 114) inside the command
record itself (layout in [write-flows.md](write-flows.md#0e-writecommand-body-x2)).
Hisense's 61 commands used two NEC address pairs (`D:0 S:191` and
`D:4 S:251`) and varied only `F`. The hub turns the text into the signal,
so the cloud catalog is a database of (protocol, parameters) per button,
the same kind of data open IR databases publish (device / subdevice /
function per protocol, e.g. irdb).

### Checked against a manufacturer's IR list `[X2✓]`

AWOL Vision publishes its Aetherion projectors' IR list (NEC, system code
`0x00CE`, one key value per button; `testdata/x2/aetherion-ir-list.json`).
Sofabaton built the "Aetherion Max" profile from it, and the app wrote all
26 of its commands as `P:NEC R:38400 D:0 S:206 F:<key value>`: the system
code's high byte is `D`, its low byte `S`, the key value `F`, in decimal
(`capture.TestX2Aetherion`). Hisense's `D:0 S:191` is its system code
`0x00BF` the same way. Power on, Power off, HDMI 1 to 3 and Home, sent by
the hub, all worked on the projector. Sofabaton left 9 of the list's 35 keys out of the
profile (Focus, Dashboard, LOW POWER, DP, Composite, Free TV, Favorite and
two key combinations).

So an NEC device's profile can be built from its published IR list alone.
We did it for one key: `sbx add-ir` wrote Focus (`F:217`, missing from
Sofabaton's profile) from the list, and the projector responded
([write-flows.md](write-flows.md#adding-one-command-to-a-device-x2)).

### Command codes are label ids `[X2✓]`

The 6-byte code of each command is the same across brands for the same
label: Power on `1713`, Power off `1718`, Home `07C7`, Netflix `0D36`, Ok
`002A`, Mute `006A` on both the Hisense and the Aetherion. Labels spelled
differently get different codes (`Navigate_up` `002E`, `Navigate up`
`4FAF`), so the code looks like an id in Sofabaton's dictionary of key
names, not part of the signal. Buttons and macros refer to commands by it.

Open: which protocol names the X2 renders (upstream photon-verified many),
whether the catalog code id and the per-command codes must be real catalog
ids, and what a learned (raw) code looks like in the same write.

## More protocols `[ref]`

Upstream played descriptors on an X2 into a second hub learning IR and
decoded the light (IrScrutinizer's `x2-protocols.md`, 2026-09-03). The
forms it found the X2 sends exactly, with irdb's device (D), subdevice
(S) and function (F), are the ones profiles write:

| Protocol | Descriptor | Note |
|---|---|---|
| NECx | `P:NECx R:38400 D S F` | Samsung32 is NECx with S = D: the hub's `P:Samsung32` is silent |
| Sony12, Sony15 | `P:Sony12 R:40000 D F MUL:2` | `MUL:2` is two repeats (three frames) |
| Sony20 | `P:Sony20 R:40000 D S F MUL:2` | |
| JVC | `P:JVC R:38000 D F` | |
| RC6 | `P:RC6 R:36000 D F MUL:1` | mode 0 |
| Samsung20 | `P:Samsung20 R:38000 D S F` | F:2 came out as 222 (open) |
| Panasonic | `P:Panasonic R:37000 C0:2 C1:32 D S F CHECKSUM:<D^S^F>` | the catalog adds `MUL:1`, with which the hub sent nothing |

Also verified there, not in profiles yet: RC6-6-20, Samsung36,
Samsung-SMT-G, Logitech, DirecTV1, Bose (`F` alone), Sharp (odd D, as
`P:Sharp2`) and SharpDVD. RC5 sent only from some states, and the
guessed Kaseikyo form sent nothing. None of these are verified on our X2
yet: each will be once a device of ours speaks it.

## Learned (raw) codes `[X2✓]`

The Projector/Google TV device (a Bluetooth device, deleted since) had
one learned IR command, "Power (IR)": command type `0x0D` (the IR class,
as descriptor commands), code all zeros, and as its signal 1,132 bytes of
timings (`testdata/x2/bluetooth-projector.yaml`, from
`dump-2026-11-01.json`). The layout is upstream's, which it validated by
playing codes on one hub and learning them on another
(`blob_decoders.build_raw_ir_blob_body`):

| Offset | Bytes | Value | Meaning |
|---|---|---|---|
| 0 | u16 BE | 1120 | the timings' length in bytes |
| 2 | `00 00 00 00` | 0 | format field (a descriptor has `00 00 11 00` here) |
| 6 | u16 BE | 26445 | carrier in Hz (a descriptor has 38000 here) |
| 8 | u32 BE each | 7525, 4503, 453, 557, … | mark, space, mark, space… in µs |
| end | `00 00 00 00` | | terminator |

280 timings: an NEC-like frame (a 7.5 ms mark, 4.5 ms space, then bits of
about 450-560 µs marks and 557 or 1,688 µs spaces) and its repeats,
ending in a 200 ms space; upstream gives a signal ending on a mark a 40
ms one.

Written back unchanged to a new device with `WriteCommand` (the IR
class, label "Power (IR)"), the hub took it, read it back byte for byte,
and, sent with `Run`, turned the projector on (2026-11-04). So raw codes
are written like any other command, with this signal: `record.RawIR`
encodes them, `irgen` renders protocols the X2 has no descriptor for
(RC5, RCA, Pioneer, NEC42, NEC with 16-bit commands, Kaseikyo of other
vendors) as timings, and `irfile` reads Flipper's raw signals and Pronto
codes into them.

## Learning `[X2✓]`

The app learns a code into a device as follows (2026-11-07, through the
proxy: `testdata/x2/app-learn-ir.jsonl`):

1. App → hub `0x04`, empty payload. The hub acks with `0x03 [00]`.
2. For each attempt it cannot read the hub sends `0x68`, empty, and keeps
   listening. A key held down failed every time; one brief press worked.
3. The code arrives as pages of family `0x06` whose body is a command
   write (as family `0x0E`), of device 0, command 0, type 0 and no
   label, its data a raw IR signal (above).
4. The app writes those same bytes with `0x0E`, with the device, the IR
   class and the label filled in, then sends `0x65 [device, 01]` and the
   device's command order (`0x61`). It sent no `0x05` afterwards.

From the app's code (`LearnDialog`): closing the dialog without a code
sends `0x05`, empty, to leave learning; a `0x05` from the hub (the
window closed) has the app arm it again with `0x04`; a learned code of
fewer than 10 timings is refused and learning armed again. There is no
timeout in the app. Ours, armed and left alone for 150 s, sent nothing:
the window stayed open, and our `0x05` got no answer, the hub carrying on
as before (2026-11-07). `[X2✓]` for what was seen, `[code]` for the rest.

RF learning, from the app's code only (`RfLearnDialog`) `[code]`: `0x70
[01, f]` starts it and `0x70 [00, f]` stops it, `f` 0 for 433 MHz and 1
for 315 MHz; the hub answers as for IR (`0x06` pages of a command write,
`0x68`, `0x05`), and status `0x11` to the start is an error the app ends
learning on. The app gives the command class `0x1D` (433 MHz) or `0x1E`
(315 MHz), the data as the hub sent it, and makes RF devices of type 17.
A second reading of the app's code (2026-11-12) agrees, and adds: status
`0x11` means the hub is busy syncing a remote or repeater; any non-empty
code is taken (no 10-timing minimum); RF codes come only from learning
(no catalog); `0x0F` (PlayBlob) tries one. The X2, and the X1S from
firmware 4, learn RF. `hub.Client.LearnRF` and `sbx learn -rf 433` do it
as the app does; not tried yet, for want of an RF remote.

The receiver's Up came out as 200 timings at 38,659 Hz: the Denon code
`D:4 S:1 F:27` twice, which its catalog profile holds as `P:DenonK …
F:27 CHECKSUM:240`, with marks about 10% short and the header 4% short.
`hub.Client.Learn` arms learning and returns the data; `sbx learn` adds
it to a device. It learned the receiver's Down (F:28) on the first
press. A device without commands answers a command-list request with
status `0x07`.

## Why this matters

Generating either form locally from open IR databases removes the dependency
on the cloud catalog for "known" devices (limitation L3, research R3).

## TODO

- [x] Raw blob byte layout (see Learned codes)
- [ ] Save pages (family `0x0E`) incl. trailing checksum
- [ ] Replay (family `0x0F`) — note: plays ignore X2 emitter routing `[ref]`
- [ ] Token/field table for `P:` with our own verification status

## Denon `[X2✓]`

Sofabaton's catalog profile for our Denon AVR-X8500H holds 71 commands:
69 are `P:DenonK R:37000 C0:84 C1:50 C2:0 D:4 S:1 F:<function>
CHECKSUM:<c>`. C0/C1 are Denon's Kaseikyo manufacturer code 0x3254,
D/S the chart's device and subdevice (4/1: main zone, remote ID 1), F the
chart's function, and the checksum is `(D·16 ^ S ^ F·16 ^ F>>4) & 0xFF`,
which reproduces all 69 (`profile.TestDenonK`,
`testdata/x2/receiver-descriptors.txt`). The other two (power on and
off) are `P:Sharp1 R:38000 D:1 F:195 C0:1` and `P:Sharp R:38000 D:1 F:61
C0:0 D_CHECKSUM:17 F_CHECKSUM:194 C1:1 D0:17`: Denon's older 15-bit
protocol.

### Denon 15-bit as `P:Sharp` `[X2✓]`

Denon's 15-bit frame is 5 address bits, 8 function bits and 2 extension
bits, LSB first, then a repeat with the function and extension inverted.
The hub's `P:Sharp` fields read the same bits **one bit later**: `C0` is
the address's low bit, `D` the other four address bits and the function's
low bit, `F` the other seven function bits; `D_CHECKSUM` (= `D0`) and
`F_CHECKSUM` are the same for the repeat frame (whose extension bit makes
`F_CHECKSUM`'s top bit 1). `C1` is 1 in every code seen.

So the catalog's Power Off is the chart's Main Zone Off, 2/122, and its
`P:Sharp1 … F:195` Power On is the *repeat* half of Main Zone On (2/121)
sent as a code; it did nothing on our receiver. Codes we rendered this
way from the chart (2/121, 2/240, 2/241, 2/242) all worked on our
AVR-X8500H, as did the Denon-K Main Zone On/Off we derived (4/1/6 and
4/1/7). Profiles write them as `ir: {protocol: Denon, device: 2}`.

Denon's "IR Command Chart" (summer 2016) gives remote ID 2-4 the same
functions as ID 1 plus 1024, on subdevice 2; that names the ID 1
functions the chart lists only as 15-bit codes (cursor 27-30, Enter 31,
volume 368-370) `[inferred]`.
