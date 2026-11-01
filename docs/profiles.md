# Device profiles

A profile is a YAML file describing a device's commands, with comments,
kept by us rather than on the hub (the hub has nowhere to store
comments). `profile` reads and writes them; `sbx` uses them:

```
sbx new -template ir/awol-vision/aetherion -o aetherion.yaml   # from a template
sbx export -hub IP -device 4 -o aetherion.yaml   # back up a device
sbx apply  -hub IP -f aetherion.yaml -n          # show what would change
sbx apply  -hub IP -f aetherion.yaml             # change it (asks first)
sbx apply  -hub IP -f new.yaml -create           # make it a new IR device
sbx remove-device -hub IP -device "ZZ Test"      # delete a device
```

`apply` compares the file with the hub and changes only what differs:
renames and new signals rewrite a command in place (same id, same code, so
buttons and macros that use it keep working), missing commands are
added, and commands the file lacks are kept unless `-prune` is given.
Commands are matched by name, then by name regardless of case, then by
signal (so a renamed command is recognized; names go first because
catalog profiles often give several commands one code). The order of the commands
in the file is the order of the device's page on the remote; `apply`
rewrites it when it differs. Keys and power macros are written when
they differ; keys the file does not bind are left as they are.

A `device:` (or `brand:`) different from the hub's renames the device in
place: activities refer to devices by id, so they keep it. (On our X2,
2026-10-31, AWOLVision became Aetherion this way.)

`-create` makes the profile's device as a new IR device instead, in the
order the app uses: the device, its commands, keys, idle behavior and
power macros, the commit, then its inputs. It refuses a name the hub
already has.

## Format

```yaml
device: AWOLVision          # the device's name on the hub
brand: AWOL Vision
from: ir/awol-vision/aetherion   # the template it was made from, if any
id: 4                       # the device's id on the hub (or -device)
icon: Projector             # the remote's icon and color, by name or
color: grey                 # number; left out, the hub's stay
ir: {protocol: NEC, system: 0x00CE}   # lets commands give just a code
commands:
  - {name: Power On, code: 0xE5}
  - {name: Focus, code: 0xD9, comment: 'opens the focus screen'}
  - {name: Other, descriptor: "P:NEC R:38400 D:4 S:251 F:114"}
  - {name: Learned, data: "00a4..."}  # the hub's bytes, for codes the
                                       # other forms cannot express
power: {on: Power On, off: Power Off}  # the POWER_ON and POWER_OFF macros
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
macros are a single command; `export` writes it then.

`inputs` answers the app's "does this device switch inputs?": the
commands that select each input, in order, for activities to pick from.
`apply` writes the inputs page (keeping the hub's input labels when only
they differ), sets the device's input mode, and syncs the device to the
remotes; with `power` it also marks the device's power as set up, as the
app does. Leaving `inputs` out leaves the hub's inputs as they are;
`inputs: []` says the device switches no inputs (an empty page and input
mode 2, as the app writes when told the device needs none).

## Templates

A template is a device's profile as we maintain it, for any hub: read-only,
without the hub's id or the device's name, headed by `template:` with the
model. `sbx new` makes a profile of one, to edit (names, keys, macros) and
apply; `apply` refuses a template itself.

```
sbx templates                                       # the templates sbx carries
sbx new -template ir/denon/avr-x8500h -name Receiver -o receiver.yaml
sbx new -template my-tv.yaml -o tv.yaml             # a template file
```

The profile keeps the template's comments; its `template:` line becomes
`from:` (where it came from) and `device:` (the `-name`, or the model).
A profile is a copy: a later fix to the template does not reach it.

[`templates/`](../templates) holds them by transport (`ir`, later
`bluetooth` and `network`), then maker: the AWOL Vision Aetherion
(`ir/awol-vision/aetherion`), written from AWOL's published IR list, the
Denon AVR-X8500H (`ir/denon/avr-x8500h`) from Denon's IR command chart, and
the Ugoos AM6B Plus (`ir/ugoos/am6b-plus`) from CoreELEC's remote
configuration. `sbx` carries them, so they work without the repository.

## Activities

`sbx export -activity NAME` writes an activity's profile: its devices and
the input each is switched to, its keys, its power macros and its
other macros. `sbx apply -f` takes it back, as for devices (it tells
them apart by the first key), and `-create` makes a new activity:

```
sbx export -activity "Watch Blu-ray" -o blu-ray.yaml
sbx apply -f blu-ray.yaml -n            # what would change
sbx apply -f movie-night.yaml -create   # a new activity
sbx remove-activity -activity "Movie Night"
```

`apply` writes the members (confirmed to the hub, then the power
macros, from which the hub takes them, then the record, as the app
does), the power macros, the activity's own macros (matched by
name; new ones get their ids from the hub and go on the remote's list in
the profile's order), the keys, and syncs the activity. A different
`activity:` renames it, and a different `icon:` or `color:` rewrites
the record. Macros the profile lacks stay unless `-prune`. Leaving out
`power_on` or `power_off` keeps the hub's, with the devices' inputs; a new
activity, or one whose devices change, gets the app's instead: each
device's power macro a second apart, then each device's input.

```yaml
activity: Watch Blu-ray
id: 103
icon: Media player              # as for devices
color: blue
devices:                        # in the hub's member order
  - {device: Receiver, input: Input Blu-ray}   # a command of its inputs
  - {device: Blu-ray}           # not switched
buttons:                        # "Device: Command", or a macro's name
  Vol+: 'Receiver: Volume Up'
  Play: 'Blu-ray: Play'
  Menu: {short: 'Blu-ray: Menu', long: Movie Mode}
power_on:
  - power on: Receiver          # runs the device's POWER_ON macro
  - wait: 1                     # seconds
  - input: Receiver             # switches it to its input above
  - Blu-ray: Home               # any device's command
power_off:
  - power off: Receiver
macros:                         # the activity's own, across its devices
  - name: Movie Mode
    steps:
      - Receiver: Movie
      - wait: 2
      - Blu-ray: Play
favorites:                      # commands on the remote's list, after the
  - 'Receiver: Movie'           # macros, in this order
  - 'Blu-ray: Top Menu'
```

Favorites the profile lacks stay unless `-prune` (`favorites: []` with
`-prune` removes them all); leaving `favorites:` out leaves the hub's
alone. A favorite is a device command with an id up to 199, the most
the hub takes; deleting one takes the hub several seconds.

### Icons and colors

Names are matched regardless of case, spaces and hyphens; a number is
the hub's id. `export` writes names. These are the X2's (from upstream's
drawings of them, checked against our hub's devices).

| Id | Icon | Id | Icon | Id | Icon |
|---|---|---|---|---|---|
| 1 | Monitor | 16 | Netflix | 31 | Wall switch |
| 2 | Picture | 17 | Media player | 32 | Presentation screen |
| 3 | Music | 18 | Curved TV | 33 | Party |
| 4 | Film reel | 19 | Download | 34 | Sleep |
| 5 | TV | 20 | Satellite dish | 35 | DAZN |
| 6 | Gamepad | 21 | Disc | 36 | DIRECTV |
| 7 | Game controller | 22 | Set-top box | 37 | Disney |
| 8 | Router | 23 | Cassette deck | 38 | Hulu |
| 9 | Wifi router | 24 | Front projector | 39 | NVIDIA |
| 10 | Projector | 25 | Smart speaker | 40 | Prime Video |
| 11 | Receiver | 26 | Air conditioner | 41 | PS |
| 12 | Pendant lamp | 27 | Desk fan | 42 | R |
| 13 | Apple TV | 28 | Ceiling fan | 43 | SAM |
| 14 | Roku | 29 | Table lamp | 44 | Xbox |
| 15 | Fire TV | 30 | Power outlet | 45 | YouTube |

Colors are the X2 palette, named for the nearest common color:

| Id | Color | App's | Id | Color | App's |
|---|---|---|---|---|---|
| 1 | grey | `#7F7F7F` | 7 | blue | `#00A8E0` |
| 2 | red | `#F16161` | 8 | indigo | `#717FB4` |
| 3 | navy | `#2E385E` | 9 | cyan | `#56CBCB` |
| 4 | purple | `#8171B4` | 10 | beige | `#BCAD90` |
| 5 | green | `#64BB80` | 11 | light blue | `#6EACEB` |
| 6 | pink | `#E39E89` | 12 | lavender | `#97A5DB` |


A command step may add `hold: N`, the step's duration byte as the hub
keeps it; what it does is not known. `export` drops the duration bytes of
`power on` steps (the hub has 1 for IR devices and 0 for Bluetooth ones),
so an exported power macro is not yet a byte-exact copy.

## Device macros

A device's profile can have macros of its own commands:

```yaml
macros:
  - name: 3D On                 # at most 27 characters
    steps:
      - Menu                    # one of the device's commands
      - {wait: 2}               # seconds
      - {send: OK, hold: 3}     # with the hub's duration byte
```

The remote cannot show a device's own macros (every one appears as the
same blank entry on its page, see
[write-flows.md](protocol/write-flows.md#device-macros-x2)), so `apply`
copies them into every activity with the device, where they are listed
by name, and keeps the copies in step with the profile: a new macro is
added to each activity's list, a changed one rewritten, and, with
`-prune`, one the profile lacks is removed. Leaving `macros` out leaves
the copies as they are. Each copy carries the device it belongs to, so:

- an activity's keys run one as `'Device: Macro'`, like a command;
- an activity's profile leaves the copies alone, `-prune` too, and its
  export lists them under the device's profile, not the activity's
  `macros`;
- an activity a device joins gets the device's macros, and one it leaves
  loses them;
- `export -device` reads the macros back from the copies.

The hub has the macros only in the activities: a device in none keeps
them in its profile alone, until `apply` runs after it joins one. A macro
cannot share a name with one of the device's commands, nor with a macro
of its own of an activity with the device.

## Not yet

Unbinding a key, the device's category, Bluetooth devices, and
protocols other than NEC, Denon-K and Denon for `code`. See
[ROADMAP.md](../ROADMAP.md).
