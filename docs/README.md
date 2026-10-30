# Sofabaton Go — documentation

Our own documentation of the Sofabaton hub protocol and of this library.
It is written from scratch; the upstream project
(`m3tac0de/home-assistant-sofabaton-x1s`, MIT) is used as a **reference**
and cited, never copied wholesale.

## Confidence tags

Every non-trivial protocol fact carries one tag:

| Tag | Meaning |
|---|---|
| `[X2✓]` | Confirmed by us on our X2 hub (link a capture in `testdata/captures/`) |
| `[ref]` | Taken from upstream docs/code/bench notes, not yet confirmed by us |
| `[ref:X1/X1S]` | Upstream validated it only on X1/X1S — X2 behaviour unknown |
| `[guess]` | Our inference; needs a capture |
| `[unknown]` | Known gap |

When a `[ref]` fact is confirmed on the X2, change it to `[X2✓]` and add the
capture. When we contradict upstream, say so explicitly.

## Index

| Doc | Content |
|---|---|
| [protocol/frame-format.md](protocol/frame-format.md) | Frame layout, checksum, deframing |
| [protocol/connection.md](protocol/connection.md) | Discovery, CALL_ME, TCP session, proxying |
| [protocol/hub-variants.md](protocol/hub-variants.md) | X1 / X1S / X2 differences |
| [protocol/records.md](protocol/records.md) | Record layouts (device body, commands, keymaps, macros, inputs) |
| [protocol/write-flows.md](protocol/write-flows.md) | Create/update sequences and acks |
| [protocol/ir-codes.md](protocol/ir-codes.md) | IR blob formats: raw timing, Pronto, X2 `P:` descriptive |
| [research/README.md](research/README.md) | Research tracks R1–R5 and capture log |

Library docs (package overviews, examples) live in Go doc comments; this
directory is for protocol knowledge and design notes.
