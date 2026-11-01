# Device profiles

A profile is a YAML file describing a device's commands, with comments,
kept by us rather than on the hub (the hub has nowhere to store
comments). `profile` reads and writes them; `sbx` uses them:

```
sbx export -hub IP -device 4 -o aetherion.yaml   # back up a device
sbx apply  -hub IP -f aetherion.yaml -n          # show what would change
sbx apply  -hub IP -f aetherion.yaml             # change it (asks first)
sbx apply  -hub IP -f new.yaml -create           # make it a new IR device
sbx remove-device -hub IP -device "ZZ Test"      # delete a device
```

`apply` compares the file with the hub and changes only what differs:
renames and new signals rewrite a command in place (same id, same code, so
buttons and sequences that use it keep working), missing commands are
added, and commands the file lacks are kept unless `-prune` is given.
Commands are matched by name, then by name regardless of case, then by
signal (so a renamed command is recognized; names go first because
catalog profiles often give several commands one code). The order of the commands
in the file is the order of the device's page on the remote; `apply`
rewrites it when it differs. Keys and power sequences are written when
they differ; keys the file does not bind are left as they are.

A `device:` (or `brand:`) different from the hub's renames the device in
place: activities refer to devices by id, so they keep it. (On our X2,
2026-10-31, AWOLVision became Aetherion this way.)

`-create` makes the profile's device as a new IR device instead, in the
order the app uses: the device, its commands, keys, idle behavior and
power sequences, the commit, then its inputs. It refuses a name the hub
already has.

## Format

```yaml
device: AWOLVision          # the device's name on the hub
brand: AWOL Vision
id: 4                       # the device's id on the hub (or -device)
ir: {protocol: NEC, system: 0x00CE}   # lets commands give just a code
commands:
  - {name: Power On, code: 0xE5}
  - {name: Focus, code: 0xD9, comment: 'opens the focus screen'}
  - {name: Other, descriptor: "P:NEC R:38400 D:4 S:251 F:114"}
  - {name: Learned, data: "00a4..."}  # the hub's bytes, for codes the
                                       # other forms cannot express
power: {on: Power On, off: Power Off}  # the POWER_ON and POWER_OFF sequences
inputs: [HDMI 1, HDMI 2]               # the commands that select each input
                                       # ([] for a device without inputs)
buttons:                               # the remote's keys on the device's page
  OK: Other
  Back: {short: Other, long: Focus}    # a long press runs another command
```

Each command has exactly one of:

| Field | Meaning |
|---|---|
| `code` | key value in the device's `ir` protocol. NEC (`ir: {protocol: NEC, system: 0x00CE}`): the hub's descriptor is `P:NEC R:38400 D:<system high byte> S:<system low byte> F:<code>` ([ir-codes.md](protocol/ir-codes.md)). DenonK (`ir: {protocol: DenonK, device: 4, subdevice: 1}`): the function number of Denon's IR command charts, rendered as `P:DenonK R:37000 C0:84 C1:50 C2:0 D:4 S:1 F:<code> CHECKSUM:<c>` with the Kaseikyo check byte. Denon (`ir: {protocol: Denon, device: 2}`): the chart's 15-bit codes, which the hub writes as `P:Sharp` ([ir-codes.md](protocol/ir-codes.md#denon-15-bit-as-psharp-x2)). A command can give its own `ir` when a device mixes protocols |
| `descriptor` | a descriptive IR code as the X2 stores it |
| `data` | the signal bytes as the hub holds them (hex); can be kept and renamed, not added |

`export` takes the protocol and parameters most of a device's commands
share as the device's `ir` and writes those commands as `code`s, the rest
as `descriptor`s, and `data` for anything else. Codes keep the base they
are written in (NEC lists use hex, Denon's decimal).
Unknown fields are errors.

`buttons` keys are the remote's key names: `Power`, `Up`, `Down`, `Left`,
`Right`, `OK`, `Back`, `Home`, `Menu`, `Vol+`, `Vol-`, `Mute`, `Ch+`,
`Ch-`, `Play`, `Pause`, `Rewind`, `Forward`, `Guide`, `DVR`, `Exit`, `Red`,
`Green`, `Yellow`, `Blue`, `A`, `B`, `C`, `0`-`9`, `-`, `Enter`, and `Power
On`/`Power Off` (rows the app binds to the device's on and off commands;
not keys). Case does not matter. `power` takes effect only when both
sequences are a single command; `export` writes it then.

`inputs` answers the app's "does this device switch inputs?": the
commands that select each input, in order, for activities to pick from.
`apply` writes the inputs page (keeping the hub's input labels when only
they differ), sets the device's input mode, and syncs the device to the
remotes; with `power` it also marks the device's power as set up, as the
app does. Leaving `inputs` out leaves the hub's inputs as they are;
`inputs: []` says the device switches no inputs (an empty page and input
mode 2, as the app writes when told the device needs none).

[`profiles/`](../profiles) holds the profiles we maintain, starting with
the AWOL Vision Aetherion, written from AWOL's published IR list, and a
Denon AVR-X8500H from Denon's IR command chart, and the Ugoos AM6B Plus
from CoreELEC's remote configuration.

## Activities

`sbx export -activity NAME` writes an activity's profile: its devices and
the input each is switched to, its keys, its power sequences and its
other sequences. Applying activity profiles waits for a capture of the X2
app writing activities; the format may still change.

```yaml
activity: Watch Blu-ray
id: 103
devices:                        # in the hub's member order
  - {device: Receiver, input: Input Blu-ray}   # a command of its inputs
  - {device: Blu-ray}           # not switched
buttons:                        # "Device: Command", or a sequence's name
  Vol+: 'Receiver: Volume Up'
  Play: 'Blu-ray: Play'
  Menu: {short: 'Blu-ray: Menu', long: Movie Mode}
power_on:
  - power on: Receiver          # runs the device's POWER_ON sequence
  - wait: 1                     # seconds
  - input: Receiver             # switches it to its input above
  - Blu-ray: Home               # any device's command
power_off:
  - power off: Receiver
sequences:                      # the activity's own, across its devices
  - name: Movie Mode
    steps:
      - Receiver: Movie
      - wait: 2
      - Blu-ray: Play
```

A command step may add `hold: N`, the step's duration byte as the hub
keeps it; what it does is not known. `export` drops the duration bytes of
`power on` steps (the hub has 1 for IR devices and 0 for Bluetooth ones),
so an exported power sequence is not yet a byte-exact copy.

## Not yet

Applying activity profiles, unbinding a key, device sequences of more
than one command (per-device ones, which `sbx` will copy into every
activity with the device), the device's category and icon, activities, Bluetooth
devices, and protocols other than NEC, Denon-K and Denon for `code`. See
[ROADMAP.md](../ROADMAP.md).
