# Research

Tracks from `PLAN.md` §7. Each finding: date, hub + firmware, app version,
capture file in `testdata/captures/`, conclusion, and which doc was updated.

| Track | Question | Status |
|---|---|---|
| R1 | Bluetooth device create: what goes into code_id / tail byte 23 / keymaps? | not started |
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

_(empty)_
