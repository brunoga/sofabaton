# sofabaton

A Go library and command-line tool for **Sofabaton X1, X1S and X2** universal
remote hubs, talking to the hub directly over its local network protocol.

The goal is full local control of the hub's configuration: create devices
from scratch (IR, Bluetooth, RF, Wi-Fi), add commands, assign buttons, build
macros and activities, and sync them to the remote — the foundation for a
better management app than the official one.

> **Status: early.** The library reads a hub's configuration (devices,
> activities, commands, keymaps, macros, inputs); writing and the app proxy
> are next. Most of it is verified against captures from upstream's X1 and
> X1S hubs, not yet against a live X2. See [PLAN.md](PLAN.md).

> This is an unofficial project and is not affiliated with or endorsed by
> Sofabaton. The protocol is reverse-engineered and may change with hub
> firmware. Writing to a hub changes its configuration: keep a backup.

## Layout

| Path | Content |
|---|---|
| `wire/`, `record/`, `codec/`, `plan/` | Pure protocol code: frames, records, IR/Wi-Fi payloads, write plans |
| `transport/`, `session/`, `hub/` | Discovery, the hub connection, the public client |
| `proxy/`, `capture/`, `hubsim/` | Official-app proxy, traffic captures, a simulated hub for tests |
| `cmd/sbx/` | Command-line tool |
| `docs/` | The protocol as we understand it, with a confidence level per fact |
| `testdata/` | Captured frames and generated test vectors |

Packages appear as the phases in [PLAN.md](PLAN.md) land.

## Using sbx

Close the Sofabaton app on every phone first: a hub serves one client at a
time and does not answer discovery while the app is connected.

```sh
sbx discover                          # hubs on the local network
sbx info -hub 192.168.1.50            # identity: model, firmware, name
sbx dump -hub 192.168.1.50 -o hub.json   # full configuration, read-only
sbx dump -hub 192.168.1.50 -v         # … logging every frame to stderr
```

`-hub` is optional: without it sbx uses `$SBX_HUB` or, if that is unset,
the hub discovery finds when exactly one answers (`sbx info`, `sbx list`).

`dump` only reads. It keeps every reply's raw bytes next to the decoded
view, so it is a lossless backup.

To watch what the official app does, put the proxy between them:

```sh
sbx proxy -hub 192.168.1.50 -capture app.jsonl -v
```

The proxy takes the hub's connection, advertises itself as the hub (mDNS for
the Android app, the NOTIFY_ME broadcast for iOS) and relays every frame
between the app and the hub, appending each to the capture. Lines typed while
it runs become notes in the capture ("now adding a Bluetooth device").

## Building

```sh
go build ./cmd/sbx
```

## Testing

```sh
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
