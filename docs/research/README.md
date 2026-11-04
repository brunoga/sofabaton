# Research

Tracks from `PLAN.md` §7. Each finding: date, hub + firmware, app version,
capture file in `testdata/captures/`, conclusion, and which doc was updated.

| Track | Question | Status |
|---|---|---|
| R1 | Bluetooth device create: what goes into code_id / tail byte 23 / keymaps? | one real Bluetooth record read (2026-10-30) |
| R2 | RF device create: class code and record shapes | not started |
| R3 | Local IR catalog → raw + `P:` generation | not started |
| R4 | App ↔ cloud catalog API (optional) | endpoints read from the APK (2026-11-04) |
| R5 | Device category semantics (type, icon, input_mode, power_style) | type values read from the APK (2026-11-04); semantics open |

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

### 2026-11-04 — the app's APK: device types and the cloud catalog

App: Sofabaton (`www.romateck.com.u1`) 4.1.6, pulled from our phone with
`adb`; read statically (resources, and the dex code disassembled with
androguard), never run. The APK is not in the repo.

**Device types** (record byte 8, `profile.Category`). The app bundles
Kookong's IR SDK; its `com.hzy.tvmao.ir.Device` numbers device types: 1
STB, 2 TV, 3 BOX (network/streaming box), 4 DVD, 5 AC, 6 PRO
(projector), 7 PA (amplifier), 8 fan, 9 SLR camera, 10 light, 11 air
cleaner, 12 water heater, 13 car multimedia, 14 footbath, 15 computer
desk. Our X2's app-made devices match (2 TV, 4 Blu-ray, 6 projector, 7
receiver). The app's own `SelectDeviceTypeActivity` gives Bluetooth
devices type `"15"` and Wi-Fi ones `"16"` (`TypeBean(etype_id, name,
logo)`), so 15 is Bluetooth, not Kookong's desk. The list the app shows
for IR devices comes from the cloud (`app/getTypes`), its ids those types.

**Cloud catalog.** Base `https://api.furrykid.com/pro/`
(`www.thl.mvp.base.Urls`), every request an OkGo `POST` of form
parameters, with the header `resource: android`. The device database:

| Endpoint | Parameters | Use |
|---|---|---|
| `app/getTypes` | — | the device types (IR) |
| `app/getAllXBrands`, `app/getAllBrands` | `language` | brands (X series, all) |
| `app/getBrandsByType` | `etype_id` | brands of a type (Bluetooth: 15) |
| `app/getTypesByBrandId` | `brand_id` | a brand's types |
| `app/getAllModels` | `brand_id` | a brand's models |
| `app/getCodeMsg` | `etype_id`, `brand_id`, `emodel_id`, `ecode_id`, `shortID`, `saveState`, `isU1` | a code set's keys and codes |
| `app/getCodeMsg2` | `ecode_id`, `codeFrom` | the same by code id (editing keys) |
| `app/getFixCode`, `app/filterFixCode` | `ecode_id`, `key_id`, `type` | single codes |
| `app/getBtCodeId`, `app/getBt` | — | Bluetooth profiles |

Others there: backups (`app/addBackup`, `app/listBackup`, …), firmware
(`app/getFirmwares`, `app/checkFirmwareUpgrade`), accounts. The Kookong SDK
has its own API (`http://sdkapi.kookong.com/m/…`: `brands`, `remotes`,
`irs`, `search`, `rctestkey`, `filterrc`), keyed by an app key from its
native library. None of these were called: whether they answer without
the app's session is untested.

**TLS.** The app's HTTP client trusts any certificate
(`HttpsUtils.getSslSocketFactory()` without arguments, and
`UnSafeHostnameVerifier`), so an intercepting proxy (mitmproxy) sees its
catalog traffic without a patched APK or a CA on the phone.
