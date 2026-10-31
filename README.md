# sofabaton

A Go library and command-line tool (`sbx`) for **Sofabaton X1, X1S and X2**
universal remote hubs, talking to the hub directly over its local network
protocol, without Sofabaton's app or cloud.

The goal is full local control of the hub's configuration: create devices
from scratch (IR, Bluetooth, RF, Wi-Fi), add commands, assign buttons, build
sequences and activities, and sync them to the remote — the foundation for a
better management app than the official one.

> **Status: 0.1, a first prerelease.** IR devices can be read, backed up,
> edited and created from scratch with YAML profiles, and the changes reach
> the remote. Everything here is verified on our X2 hub; X1 and X1S are
> modeled from upstream's captures but not tested by us. Bluetooth devices,
> activities and restoring a backup are not there yet: see
> [ROADMAP.md](ROADMAP.md).

> This is an unofficial project and is not affiliated with or endorsed by
> Sofabaton. The protocol is reverse-engineered and may change with hub
> firmware. Writing to a hub changes its configuration: keep a backup
> (`sbx dump`, `sbx export`).

## Installing

Download a binary for Linux, macOS or Windows from the
[releases](https://github.com/brunoga/sofabaton/releases), or:

```sh
go install github.com/brunoga/sofabaton/cmd/sbx@latest
```

## Using sbx

Close the Sofabaton app on every phone first: a hub serves one client at a
time and does not answer discovery while the app is connected.

Every command that talks to a hub takes `-hub IP`. Without it, sbx uses
`$SBX_HUB` or, if that is unset, the hub discovery finds when exactly one
answers. Devices are given by name or id.

### Looking

```sh
sbx discover                          # hubs on the local network
sbx info                              # identity: model, firmware, name
sbx list                              # devices and activities
sbx list commands Receiver            # a device's commands, in the remote's order
sbx list buttons "Watch Blu-ray"      # what each key runs
sbx list sequences Receiver           # sequences (macros), e.g. power on/off
sbx dump -o hub.json                  # the full configuration, read-only
```

`dump` keeps every reply's raw bytes next to the decoded view, so it is a
lossless backup. `-v` on any command logs every frame to stderr.

### Changing devices with profiles

A profile is a YAML file describing one device: its commands (as IR codes),
their order on the remote, the keys, the power-on/off commands and the
inputs. Export a device, edit the file, and apply it back; `apply` changes
only what differs.

```sh
sbx export -device Receiver -o receiver.yaml   # write a device's profile
sbx apply -f receiver.yaml -n                  # show what would change
sbx apply -f receiver.yaml                     # change it (asks first)
sbx apply -f receiver.yaml -prune              # … also removing commands the file lacks
sbx apply -f new-tv.yaml -create               # create a new IR device
sbx remove-device -device "Old TV"             # delete a device
```

```yaml
device: Receiver
brand: Denon
ir: {protocol: DenonK, device: 4, subdevice: 1}
commands:
  - {name: Main Zone On, code: 6}
  - {name: Main Zone Off, code: 7}
  - {name: Volume Up, code: 368}
buttons:
  Vol+: Volume Up
power: {on: Main Zone On, off: Main Zone Off}
```

Renaming a device or a command keeps the activities and keys that use it.
After any change `apply` syncs the device to the remote. IR codes can be
given as NEC, Denon-K or Denon (Sharp) codes, or as the hub's own
descriptors. [docs/profiles.md](docs/profiles.md) has the whole format;
[profiles/](profiles) holds the profiles we maintain (the AWOL Vision
Aetherion projector, the Denon AVR-X8500H receiver, the Ugoos AM6B Plus
media player).

### Single commands

```sh
sbx send -device Receiver -command "Volume Up"     # send a command now
sbx add-ir -device TV -label Focus -code "P:NEC R:38400 D:0 S:206 F:217"
sbx remove-ir -device TV -label Focus
sbx sync -device TV                                # refresh the remote's copy
```

### Watching the official app

To see what the official app does, put the proxy between them:

```sh
sbx proxy -capture app.jsonl -v
```

The proxy takes the hub's connection, advertises itself as the hub (mDNS for
the Android app, the NOTIFY_ME broadcast for iOS) and relays every frame
between the app and the hub, appending each to the capture. Lines typed while
it runs become notes in the capture ("now adding a Bluetooth device"). This
is how the protocol behind every write above was learned.

## Layout

| Path | Content |
|---|---|
| `wire/`, `record/` | Pure protocol code: frames, paging, the hub's records |
| `transport/`, `session/`, `hub/` | Discovery, the hub connection, the public client |
| `profile/` | YAML device profiles, IR protocols, plans and their application |
| `proxy/`, `capture/`, `hubsim/` | Official-app proxy, traffic captures, a simulated hub for tests |
| `cmd/sbx/` | Command-line tool |
| `docs/` | The protocol as we understand it, with a confidence level per fact |
| `profiles/` | Device profiles we maintain |
| `testdata/` | Captured frames, hub dumps and generated test vectors |

## Building and testing

```sh
go build ./cmd/sbx
make ci                       # every CI job: lint, race tests, fuzz smoke, cross-builds
go test -race ./...           # unit and simulated-hub tests
make fuzz FUZZTIME=1m         # every fuzz target
SOFABATON_HUB=192.168.1.50 make test-live   # against a real hub (see CONTRIBUTING.md)
```

## Credits

The protocol knowledge this library starts from comes from
[m3tac0de/home-assistant-sofabaton-x1s](https://github.com/m3tac0de/home-assistant-sofabaton-x1s)
(MIT); see [NOTICE](NOTICE).

## License

[Apache License 2.0](LICENSE).
