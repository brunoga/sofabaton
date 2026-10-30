# sofabaton

A Go library and command-line tool for **Sofabaton X1, X1S and X2** universal
remote hubs, talking to the hub directly over its local network protocol.

The goal is full local control of the hub's configuration: create devices
from scratch (IR, Bluetooth, RF, Wi-Fi), add commands, assign buttons, build
macros and activities, and sync them to the remote — the foundation for a
better management app than the official one.

> **Status: early.** The repository is being set up; nothing talks to a hub
> yet. See [PLAN.md](PLAN.md) for the roadmap.

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
