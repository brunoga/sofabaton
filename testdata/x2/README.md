# X2 captures

From the maintainer's own X2 hub (id `80f1b2962c08`), published with the
owner's consent.

| File | What | Checked by |
|---|---|---|
| `dump-fw8.json` | `sbx dump` of the whole configuration, firmware 8 | `cmd/sbx.TestDumpRoundTrip` |
| `dump-fw9.json` | the same after the update to firmware 9 (identical but for the version) | `cmd/sbx.TestDumpRoundTrip` |
| `app-firmware-update.jsonl` | `sbx proxy`: the Android app updating the hub from 8 to 9; image bytes removed | `capture.TestX2FirmwareUpdate` |

The configuration: devices Receiver, Blu-ray and Projector/Google TV (a
Bluetooth device), and activities Watch Google TV, Play Switch 2, Watch
Blu-ray and Play PC.
