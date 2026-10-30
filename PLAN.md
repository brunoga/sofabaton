# Sofabaton Go library — plan

Source of truth for the Go port of the Sofabaton X1/X1S/X2 hub protocol.
Update this file as decisions are made; keep the "Status" column honest.

## 1. Goal

An idiomatic, testable Go library (plus a small CLI) that talks to Sofabaton
hubs locally, with **creating devices from scratch** as the primary use case:
devices, commands, button assignments, macros/sequences, activities.
Later consumer: a web UI ("a better Sofabaton app").

Primary target hardware: **X2 hub + X2 remote** (owned), with the official
**Android app** available for capture. X1/X1S support is kept in the wire
model (dimensions differ) but is not validated by us.

Non-goals (for now): Home Assistant integration, the upstream persistent-cache
/ snapshot-diff editor engine, multi-hub server.

## 2. Reference material (read-only)

Upstream: `github.com/m3tac0de/home-assistant-sofabaton-x1s` (MIT), studied at
commit `5fd5887` (2026-10-30).

| What | Where (upstream) | Use |
|---|---|---|
| Protocol library (~44k lines Python) | `custom_components/sofabaton_x1s/lib/` | behaviour reference, oracle for test vectors |
| Wire docs (~6.7k lines) | `docs/protocol/*.md` | reference for our own docs |
| Bench notes, incl. app-captured flows | `docs/protocol/live-hub-testing.md` | what was validated on which hub |
| X2 descriptive IR (`P:` strings) | `IrScrutinizer/x2-protocols.md`, `sofabaton-x2.xml` | IR generator mapping |
| Captured frames | ~184 hex frames in 17 test files (`tests/test_x1_proxy.py`, `test_commands.py`, `test_opcode_handlers.py`, `test_device_create.py`, `test_phase{2,3}_real_capture.py`, …), 10 of them touch X2 | golden decode vectors |
| Codec vectors | `tests/fixtures/{ir-format,uc-hex,wifi-ip-form,wifi-name}-vectors.json` | golden codec vectors |
| Bench scripts | `scripts/hub-bench/bench_*.py` (~150) | live test procedures to port |
| Self-hosted server + web UI | `sofabaton-x-server/` (OpenAPI spec, MIT) | prior art for our web UI |

**Our docs are our own** (`docs/`), written in our words and marked with a
confidence level per fact (see `docs/README.md`). Upstream docs are cited, not
copied. When we confirm something on our X2, the doc says so.

## 3. Known upstream limitations we want to overcome

| # | Limitation | Why it exists | How we attack it |
|---|---|---|---|
| L1 | Cannot create **Bluetooth** devices | needs a per-brand profile id in the 16-byte `code_id`, never captured | capture the Android app creating BT devices through our proxy (R1) |
| L2 | Cannot create **RF** devices (no known class code) | never captured | capture app RF-device create on the X2 (R2) |
| L3 | "Known device" IR codes come from Sofabaton's cloud catalog only | app downloads codes over HTTPS; hub only stores blobs | (a) generate codes locally from open IR databases + protocol encoders (R3, primary); (b) optional: study app↔cloud API (R4) |
| L4 | Most write flows bench-validated on **X1/X1S**, fewer on X2 | upstream bench hardware | our own X2 bench suite (live tests) |
| L5 | Device category / icon / input-mode / power-style semantics only partly mapped | only a few categories captured | capture app creates per category, table-drive them (R5) |
| L6 | No general physical-button press stream | hub limitation | out of scope; Wifi Commands/Events later |

## 4. Architecture

Module: `github.com/brunoga/sofabaton`, Apache-2.0 (upstream MIT credited in
`NOTICE`). Go 1.27, stdlib-first;
only external dep expected is an mDNS library (or a small own implementation).

```
sofabaton/
  wire/        frames, checksum, deframer, opcodes+families, label encodings,
               paging wrapper. Pure, no I/O.
  record/      typed hub records with MarshalBinary/UnmarshalBinary per hub
               variant: DeviceRecord (120/210-byte body), catalog rows,
               CommandRecord, KeymapRow/ButtonBinding, MacroRecord, InputsPage,
               membership rows, favorites. Pure.
  codec/ir/    raw-timing blobs, Pronto hex, X2 descriptive `P:` strings,
               protocol encoders (NEC, NECx, Sony12/15/20, RC5, RC6, Samsung,
               Panasonic/Kaseikyo, Sharp, JVC, Denon-K …). Pure.
  codec/wifi/  wifi_ip / roku / hue / sonos / mqtt command payloads. Pure.
  plan/        write flows as data: BuildDeviceCreate(spec, variant) []Step,
               where Step = {frame, expected ack matcher, capture rules}.
               Pure → golden-testable against captured app sequences.
  transport/   discovery (mDNS + UDP NOTIFY_ME), CALL_ME, TCP accept-back.
               Exposes a FrameConn interface (ReadFrame/WriteFrame).
  session/     engine: one reader goroutine, one serialized exchange at a time,
               ack correlation, multi-frame burst assembly, retries, context
               deadlines, typed errors (ErrRejected, ErrTimeout, ErrBusy).
  hub/         public client: Dial, Catalog, Commands, Buttons, Macros, Blob,
               SendKey, CreateDevice, AddCommand, BindButton, SaveMacro,
               CreateActivity, Delete*, SyncRemote, Backup. Thin over plan+session.
  proxy/       MITM proxy (app ↔ us ↔ hub) with mDNS mirroring; write-guard
               while the app is attached.
  capture/     capture file format (JSONL: ts, direction, hex, decoded),
               recorder (hooked into proxy/session), replay.
  hubsim/      simulated hub over FrameConn: scripted from captures or a small
               state machine; used by unit/integration tests and UI dev.
  cmd/sbx/     CLI: discover, info, dump, capture, play, create-device, …
  docs/        our protocol + library docs
  testdata/    golden vectors (extracted + oracle-generated)
  tools/oracle/ Python scripts that call the upstream lib to emit vectors
```

Design rules:
- **Pure core, thin I/O.** Everything in `wire`, `record`, `codec`, `plan` is
  deterministic and table-tested. I/O lives in `transport`/`session`.
- **Plan/execute split.** A write is first a `[]plan.Step`, then executed.
  The UI can preview it; tests compare it byte-for-byte with captured app
  traffic; a dry-run mode prints it.
- `context.Context` on every blocking call; no global state; `log/slog`;
  errors are values (`errors.Is/As`), never panics on hub input.
- Hub-variant differences are data (`wire.Variant{X1,X1S,X2}` with
  dimensions, encodings), not branches scattered through code.
- Unknown bytes are preserved on read→modify→write (e.g. device tail bytes
  22–23: color, BT link mode). Records keep a raw copy for round-tripping.

## 5. Testing strategy

1. **Golden decode vectors**: extract the captured frames from upstream tests
   into `testdata/captures/*.json` with their expected decodes (script in
   `tools/`). Every record decoder must round-trip them.
2. **Oracle vectors**: Python scripts call upstream pure builders
   (`build_device_create_payload`, IR blob save pages, macro/inputs builders,
   descriptive IR, wifi payloads) over many inputs and write JSON; Go must
   produce identical bytes. Regenerated on demand, committed.
3. **Fuzzing**: `go test -fuzz` on deframer and every `UnmarshalBinary`.
4. **hubsim integration tests**: full flows (connect, catalog burst, device
   create with acks/rejects/timeouts) via `net.Pipe`, race detector on.
5. **Captured-flow tests**: for flows we capture from the Android app, assert
   our `plan` output equals what the app sent (modulo ids/timestamps).
6. **Live tests** (`//go:build live`, `SOFABATON_HUB=…`): run against our X2,
   only touching sandbox entities named `ZZ-test-*`, with a backup dump taken
   first and cleanup after. Ported from the upstream bench scripts we need.

## 6. Phases

| Phase | Content | Exit criteria | Status |
|---|---|---|---|
| P0 | Repo scaffold, go.mod, CI (vet, golangci-lint, race, fuzz smoke, cross-build), docs skeleton, vector extraction + oracle tooling | `go test ./...` green; vectors committed | done except oracle vectors (running upstream Python needs approval) |
| P1 | `wire`: frames, deframer, opcodes table, labels, paging | all captured frames deframe + checksum; fuzz clean | done: 125/125 complete captures round-trip; 3 fuzz targets |
| P2 | `record` decoders/encoders for catalog, device body, commands, keymaps, macros, inputs, blobs | golden + oracle vectors pass | in progress: entity body + device tail + catalog rows done |
| P3 | `transport` + `session` + `hubsim`; discovery & connect | connect to real X2, read banner/identity; hubsim tests | — |
| P4 | Read API in `hub`: catalog, commands, buttons, macros, blob dump, send key; **full backup dump** to JSON | complete dump of our X2, round-trip of every record | — |
| P5 | `proxy` + `capture` + `sbx capture`: Android app attaches through us | app works through proxy; JSONL captures of app flows | — |
| P6 | Writes via `plan`: IR device create, command add (IR blob save), button binding, macros, activity create, inputs, delete, remote sync; X2 live suite | create a full IR device+activity on X2 that works from the remote | — |
| P7 | `codec/ir` generators + open IR DB import (raw and X2 `P:`) | generated codes operate real equipment | — |
| P8 | Research tracks R1–R5 → close L1, L2, L5 | BT and RF device create work on X2 | — |
| P9 | Wifi classes (ip/roku/hue/sonos/mqtt), restore from backup | — | — |
| P10 | Web UI (separate plan): HTTP/WS API over `hub`, device/command/sequence editor | — | — |

P5 is intentionally early: the proxy is our main research instrument.
Until our own restore exists (P9), the upstream server/HA integration and the
app's own backup are the safety net for live write testing.

## 7. Research tracks (using our X2 + Android app)

- **R1 Bluetooth create**: through the proxy, create BT devices of 2–3 brands
  in the app; diff `0x07`/`0x08` bodies (code_id, device_type, tail byte 23,
  keymap pages). Determine if code_id is a cloud profile id we can reuse/copy.
- **R2 RF create**: create an RF (433 MHz) device in the app; capture class
  code, record body, command record shape.
- **R3 Local IR catalog**: map open databases (IRDB / Flipper-IRDB, protocol +
  device/subdevice/function) → our encoders → raw blobs (all hubs) and `P:`
  strings (X2). Validate with hub-learn loopback (X2 plays, learner captures)
  or the equipment itself.
- **R4 Cloud catalog (optional)**: static analysis of the APK (endpoints,
  request shape) and/or HTTPS interception (needs a user CA trusted by the app
  → patched APK or rooted phone). Personal-interop only; decide later.
- **R5 Category semantics**: create one device per app category; tabulate
  device_type, icon, input_mode, power_style, share_mode, default keymap.
- Every finding → `docs/` with confidence level + a capture in `testdata/`.

## 8. Open questions

- mDNS: third-party lib vs minimal own implementation (we only need
  browse + advertise for one service type).
- Do we want X1/X1S at all beyond the variant tables?
- Web UI: reuse upstream server's OpenAPI shape (and possibly its MIT
  frontend) or design our own API around `plan` previews?
