# X2 captures

From the maintainer's own X2 hub (id `80f1b2962c08`), published with the
owner's consent.

| File | What | Checked by |
|---|---|---|
| `dump-fw8.json` | `sbx dump` of the whole configuration, firmware 8 | `cmd/sbx.TestDumpRoundTrip` |
| `dump-fw9.json` | the same after the update to firmware 9 (identical but for the version) | `cmd/sbx.TestDumpRoundTrip` |
| `app-add-ir-device.jsonl` | `sbx proxy`, with notes: browsing a device and an activity, then adding a Hisense TV from the catalog and deleting it | `capture.TestX2AddIRDevice` |
| `app-add-aetherion.jsonl` | `sbx proxy`, with notes: adding "Aetherion Max" (a profile Sofabaton built from AWOL Vision's IR list), testing power, setting up inputs | `capture.TestX2Aetherion` |
| `aetherion-ir-list.json` | AWOL Vision's published IR list for the Aetherion | `capture.TestX2Aetherion` |
| `app-firmware-update.jsonl` | `sbx proxy`: the Android app updating the hub from 8 to 9; image bytes removed | `capture.TestX2FirmwareUpdate` |

The configuration: devices Receiver, Blu-ray and Projector/Google TV (a
Bluetooth device), and activities Watch Google TV, Play Switch 2, Watch
Blu-ray and Play PC.
