# Device profiles

A profile is a YAML file describing a device's commands, with comments,
kept by us rather than on the hub (the hub has nowhere to store
comments). `profile` reads and writes them; `sbx` uses them:

```
sbx export -hub IP -device 4 -o aetherion.yaml   # back up a device
sbx apply  -hub IP -f aetherion.yaml -n          # show what would change
sbx apply  -hub IP -f aetherion.yaml             # change it (asks first)
```

`apply` compares the file with the hub and changes only what differs:
renames and new signals rewrite a command in place (same id, same code, so
buttons and sequences that use it keep working), missing commands are
added, and commands the file lacks are kept unless `-prune` is given.
Commands are matched by name, then by signal (so a renamed command is
recognized), then by name regardless of case.

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
```

Each command has exactly one of:

| Field | Meaning |
|---|---|
| `code` | key value in the device's `ir` protocol. NEC: the hub's descriptor is `P:NEC R:38400 D:<system high byte> S:<system low byte> F:<code>` ([ir-codes.md](protocol/ir-codes.md)) |
| `descriptor` | a descriptive IR code as the X2 stores it |
| `data` | the signal bytes as the hub holds them (hex); can be kept and renamed, not added |

`export` writes `code`s when all of a device's commands are NEC codes of
one system code, `descriptor`s otherwise, and `data` for anything else.
Unknown fields are errors.

[`profiles/`](../profiles) holds the profiles we maintain, starting with
the AWOL Vision Aetherion, written from AWOL's published IR list.

## Not yet

Creating a device from a profile, button bindings, sequences (including
per-device ones), the remote's command order, and protocols other than
NEC for `code`.
