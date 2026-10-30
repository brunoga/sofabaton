# Research

Tracks from `PLAN.md` §7. Each finding: date, hub + firmware, app version,
capture file in `testdata/captures/`, conclusion, and which doc was updated.

| Track | Question | Status |
|---|---|---|
| R1 | Bluetooth device create: what goes into code_id / tail byte 23 / keymaps? | one real Bluetooth record read (2026-10-30) |
| R2 | RF device create: class code and record shapes | not started |
| R3 | Local IR catalog → raw + `P:` generation | not started |
| R4 | App ↔ cloud catalog API (optional) | not started |
| R5 | Device category semantics (type, icon, input_mode, power_style) | not started |

## Capture procedure (once P5 exists)

1. Close the app on all phones; start `sbx proxy --capture out.jsonl`.
2. Open the app; it should list our proxy as the hub.
3. Perform exactly one action; note the time and what you did.
4. Detach the app; annotate the capture; commit it under `testdata/captures/`.

## Log

### 2026-10-30 — first contact with our X2 (read-only)

Hub: X2, firmware 8, batch 2022-11-20, id `80f1b2962c08` (its MAC). Tools:
`sbx discover`, `sbx info -v`, `sbx dump -v` (read requests only). The dump
is kept locally, not in the repo.

- Discovery (NOTIFY_ME broadcast), CALL_ME and the TCP connect-back work as
  documented. The banner arrived as `0x1502`; marker `00`, flags `00 00`
  (upstream saw `01 00` on its X2: flags vary, as upstream warned).
- 97 replies, no errors: 3 devices, 4 activities, each device's commands,
  keymap, macros and inputs page, each activity's keymap, macros and
  members. Every one of the 43 records decodes and re-encodes to its bytes
  (`SOFABATON_DUMP=… go test ./cmd/sbx -run TestDumpRoundTrip`).
- **A Bluetooth device** (class `03`, "Projector/Google TV", brand AWOL
  Vision) with code id `e2a55f31b45545989124017b61986f7e`, holding both
  Bluetooth (`03`) and IR (`0D`) commands. Input to R1.
- Device tails: IR routing `FC 00 00 07` on all three devices, the
  Bluetooth one included; color byte 22 = `01`; byte 23 (Bluetooth link
  mode) = `01` on the Bluetooth device, `FF` on the IR ones. Upstream
  assumed `0` classic / `2` BLE without a bench; `01` is a real value.
- Activity tails start `FC 00 FC 00` and carry the color at byte 22 too
  (`05`, `02`, `07`, `0A`).
- Stored checksums: stale on device records, `00` on activity records and
  macros, valid on inputs pages.
