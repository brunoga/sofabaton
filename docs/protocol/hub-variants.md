# Hub variants

Reference: upstream `docs/protocol/hub-versions.md`.

| | X1 | X1S | X2 |
|---|---|---|---|
| `HVER` | 1 | 2 | 3 |
| Device/activity body | 120 B | 210 B | 210 B |
| Name/brand/tail slots | 30 B ASCII | 60 B UTF-16BE | 60 B UTF-16BE |
| Remote sync trigger | `0x0064` | `0x0064` | `0x0364 FF FF FF` (targetable per remote/repeater) |
| Paired remote/repeater list | — | — | `0x012E` → family `0x2F` |
| Descriptive IR `P:` | — | — | yes |
| MQTT wifi devices | — | — | yes |
| IR emitter routing (hub/ports/remotes) | — | — | yes (device tail bytes 18–21) |
| Device colors | — | — | palette 1–12 (tail byte 22) |

All `[ref]`. In Go these differences are data in `wire.Variant`.
