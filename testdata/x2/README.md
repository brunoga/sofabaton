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
| `app-activity.jsonl` | `sbx proxy`, with notes: setting up the Ugoos's power (Power for both) and no inputs, creating ZZ Activity (receiver, Aetherion, Ugoos), adding and editing a sequence, rebinding a key, removing the Ugoos, renaming and deleting the activity | `capture.TestX2Activity` |
| `dump-2026-11-01.json` | `sbx dump` before Projector/Google TV (id 1), the Aetherion paired over Bluetooth, was deleted: the Bluetooth device's records, commands (HID usages, and one IR command, "Power (IR)") and keys, for Bluetooth support | `cmd/sbx.TestDumpRoundTrip` |
| `bluetooth-projector.yaml` | `sbx export` of that Bluetooth device: its commands' data as the hub holds them | — |
| `app-add-bluetooth.jsonl` | `sbx proxy`: the Android app adding AndroidBox from the catalog's Android Box Bluetooth profile and pairing it with the AWOL projector, then reordering the activities and putting them back | `profile.TestCreateBluetooth` |
| `app-add-hue.jsonl` | `sbx proxy`: the Android app adding a Philips Hue bridge with 100 commands (on, off and 10/50/80% brightness of its rooms and lights) (2026-11-06); the bridge's user replaced by `0123456789abcdefghijABCDEFGHIJ0123456789`, the checksums made again | `profile.TestCreateHue` |
| `app-pair-phone.jsonl` | `sbx proxy`: the Android app adding the same Android Box profile and pairing it with a phone (2026-11-06): the same frames, the commands as one burst of 101 with id 0 and the catalog function ids as codes; a replay of them, after failed attempts, failed to pair as ours did until the hub was restarted | — |
| `bluetooth-androidbox.yaml` | `sbx export` of AndroidBox after that create | `profile.TestCreateBluetooth` |
| `app-clear-device-key.jsonl` | `sbx proxy`: the Android app clearing the Receiver's Up key (Customize Remote buttons, Clear assignment), then assigning it again | `capture.TestX2UnbindDeviceKey` |
| `app-firmware-update.jsonl` | `sbx proxy`: the Android app updating the hub from 8 to 9; image bytes removed | `capture.TestX2FirmwareUpdate` |

The configuration: devices Receiver, Blu-ray and Projector/Google TV (a
Bluetooth device), and activities Watch Google TV, Play Switch 2, Watch
Blu-ray and Play PC.
