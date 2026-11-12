# Matter

The X2 is a Matter device, from firmware 9: a Home app (Apple's, Google's,
others) adds it, and it offers up to eight switches ("Matter Switch 1"
to 8) whose presses the Home app's automations act on. The hub does not
control other Matter devices, and the Sofabaton app is not a Matter
controller: it shows the hub's setup code, and the Home app does the rest.

Sources: the Sofabaton app's code (version of 2026-11-12, its Matter
screens), `[app]`, and our X2, `[X2✓]`.

## The setup code `[X2✓]`

Family `0x81`, empty, asks for it. The hub answers in family `0x81` with
one byte (22 on ours: the length of the text after it), the QR code's
text (`MT:` and base-38, as the Matter specification lays it out), a
carriage return, and the manual code (`DDDD-DDD-DDDD`). The app draws the
QR code itself (ZXing) and shows the manual code without its separators.

Our X2's QR text says (decoded as the specification lays it out): version
0, vendor `0x15D6`, product `0x1234`, standard commissioning flow, and
discovery over Bluetooth LE only, as for a device not yet on a Matter
fabric (2026-11-12). `hub.Client.MatterCode` reads it; `sbx matter` draws
it in the terminal.

## The switches' device

Family `0x72`, empty, asks for it. In the app's code the reply is a
device catalog of one row (the X2 device row, family `0x72`), the device
whose commands are the switches; the app treats it as device 51
everywhere (the wireless buttons' Matter tab asks `0x5C [51, FF]`).
Before a Home app has added the hub, ours answered status `0x07` `[X2✓]`,
and the app then shows the setup code. `hub.Client.MatterDevice` reads
it, ErrNotFound until then.

Once it exists, the app uses the families every device uses `[app]`:

| Frame | What |
|---|---|
| `5C [51, FF]` | the switches (commands), at most eight shown, in id order |
| `3F [51, id]` | presses one, as a command runs |
| `0C [51, id]` then `0E` | renames one (a label of at most 20 characters) |
| `3C [51, FF]`, `3E` | the keys of the device's page bound to switches |
| `40 [51]` → `42` | the device's power key style (1 one key, 2 or 3 two) |

The app's wireless buttons can press a switch too: `79 [button, 0, 0,
device, key, device, key]` sets a button's single and double press,
`80 [button, 0, 0]` reads them and `84 [device, key]` tries one `[app]`.

## Still to see

What the device's record, class and commands hold once a Home app has
added the hub; whether it has eight switches from the start; and what a
press is in Matter (a Generic Switch event, the app's "Matter--Generic
switch", or an On/Off change).
