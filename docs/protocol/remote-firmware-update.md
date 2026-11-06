# X2 remote firmware update

The X2 remote is updated over Bluetooth Low Energy from the phone, not through
the hub, with SiFli Semiconductor's "OTA V3" protocol. Implemented by
`sifli.Update`; `sbx firmware -remote` runs it over BlueZ, and Armrest over
Android's Bluetooth. Checked on our X2's remote (2026-11-06): Armrest on a
Pixel (Android API 37) sent it firmware 9 again, and the hub reported it back
on 9 (`sbx`'s BlueZ path is not yet run against it).

Confidence tags: **[seen]** in the HCI snoop log (2026-11-06, firmware 8 to 9, LE handle 69);
**[code]** from the Android app's dex or SiFli's public sources; **[guess]** is inference.

Sources: the Bluetooth (HCI snoop) log of the Android app updating our X2's
remote from firmware 8 to 9 (2026-11-06; kept private, it holds other devices'
traffic: `testdata/sifli/remote-8-to-9.json` is its sequence of control
messages), the app's code (its bundled SiFli library, "SFOtaV3Manager" 2.1.1,
and `X2RemoteUpgradeActivity`), and SiFli's public SDK
(https://github.com/OpenSiFli/SiFli-SDK, Apache-2.0: the serial transport,
`middleware/dfu`, and the firmware packaging docs). The update protocol itself
(message ids 40-60) is not in the open SDK; its layer below comes from the log
and the app. `sifli.Update` implements the sequence below, and
`sifli.TestUpdate` checks it against the log's.

---

## 1. Finding and connecting to the remote

| Item | Value | Tag |
|---|---|---|
| Remote BLE address | `fa:ba:25:xx:xx:xx`, **public** address type. It is the same in normal mode and in OTA mode. | [seen] |
| Advertising (normal mode) | Legacy ADV_IND (event type 0x0013), empty scan response. AD: `02 01 05` (flags: LE Limited Discoverable, BR/EDR not supported), `03 19 c1 03` (Appearance 0x03C1, HID Keyboard), `09 09 "X2R_xxxx"`. The name is "X2R_" plus the last 2 address bytes in hex. | [seen] 16:54:33, 16:59:20 |
| How the app gets the address | `UpdataFirmare.mHubBean.getMac().toUpperCase()` (`X2RemoteUpgradeActivity.initData`), then `connectToMAC(mac, useHighSpeedMode=true)`. It does a direct connect through the filter accept list (`LEAddWL 00 xxxxxx25bafa`, then `LEExtCreateConn` with filter policy 1). There is no scan-and-match by name. | [seen]+[code] |
| Pairing / bonding | **None.** There is no SMP traffic and no encryption on handle 69. The remote's OTA connection is unauthenticated. | [seen] |
| Connection parameters | The app first requests 10-20 ms and gets 18.75 ms. Android's default then gives 48.75 ms. Then `requestConnectionPriority(HIGH)` gives 11.25-15 ms with latency 1, so 15 ms. In OTA mode the **remote** sends an L2CAP Connection Parameter Update Request: 15 ms / latency 0 / 6 s timeout. The phone accepts it. | [seen] 16:55:03-22, cid 5 |
| PHY | `setPreferredPhy(2M,2M)`, giving 2M/2M. | [seen]+[code] `oOOo0000.onServicesDiscovered` |
| Data length | 251-octet LL payload (DLE) in both directions. | [seen] |
| ATT MTU | The phone requests 517 and the remote answers 517. **The app caps its usable MTU at 247** (`c70`: `if mtu>247 mtu=247`), so every write is at most 244 bytes of value. | [seen]+[code] |
| App waits | It waits about 1 s after setting priority/PHY (`x00.OooO0o(1000)`), and 200 ms before `requestMtu` ("wait 200 before request mtu"). Then it enables notifications, then MTU, then the first command. It clears the GATT cache on reconnect (`refreshDeviceCache`). | [code], [seen] timings |

**Handles 66 and 67 are not the remote.** They are other bonded, encrypted devices that the OS
auto-connects. They have different random addresses, use no SiFli UUIDs, act as GATT clients of
the phone, and stay connected before and after the update. Their data is not used or reproduced
here. **[seen]**

## 2. GATT layout (identical handles in both modes, except the app service is absent in OTA mode)

| Handle | Attribute | UUID (128-bit, standard string form) | Props |
|---|---|---|---|
| 0x0001-0x0009 | GAP 0x1800: 0x2A00 name (0x0003), 0x2A01, 0x2AA6, 0x2AC9 | | |
| 0x000A-0x000D | GATT 0x1801: 0x2A05 Service Changed (0x000C), CCCD 0x000D | | |
| 0x000E-0x0013 | **Serial-transport service "sifli_sd"** | `00000000-0000-0000-6473-5f696c666973` | |
| 0x0010 | config char (unused) | `00000000-0000-0100-6473-5f696c666973` | 0x0E read/wwr/write |
| **0x0012** | **data char**: all OTA traffic, both directions | `00000000-0000-0200-6473-5f696c666973` | 0x1E read/wwr/write/notify |
| 0x0013 | CCCD of 0x0012. The app writes `01 00` (notify) with Write Request. | | |
| 0x0014-0x001B | "sifliapp" service. **Present only in normal mode.** Not used by the OTA. | `00000000-0000-0000-7369-666c69617070` | |
| 0x0016 / 0x0018 (CCCD 0x0019) / 0x001B | chars `...-0100-7369-666c69617070` (0x0E), `...-0200-...` (0x3E), `...-0300-...` (0x06) | | |

Raw little-endian bytes on the wire for the service are `7369666c695f7364 0000 000000000000`.
The app looks the service and characteristic up by UUID (`oOOo0000.onServicesDiscovered`); it
does not use fixed handles. **[seen]+[code]**

Writes from phone to remote are **ATT Write Command (0x52, without response)**, `setWriteType(1)`.
Replies from remote to phone are **notifications (0x1B)** on 0x0012. **[seen]+[code]**

## 3. Framing

### 3.1 SiFli BLE serial transport (one per ATT value) [seen]+[code: ble_profile_serial.md, c70, x00]

```
complete (flag 0)    : cate_id u8 | flag u8 = 0 | length u16le | data[length]
first fragment (1)   : cate_id u8 | flag u8 = 1 | length u16le (TOTAL message length) | data...
continuation (2)     : cate_id u8 | flag u8 = 2 | data...
last fragment (3)    : cate_id u8 | flag u8 = 3 | data...
```
- `cate_id = 0x01` (DFU category; `BLE_DFU_CATEID 0x01` in SDK `dfu_port.c`).
- The app splits with mtu = min(negotiated, 247): first fragment carries `mtu-7` = 240 data bytes,
  every following fragment carries `mtu-5` = 242. A message of 240 bytes or less (with mtu 247) is
  sent with flag 0. Example: a 557-byte data message becomes 3 writes of 244/244/73 value bytes.
- Notifications use the same format. All of the remote's replies seen were single complete frames.

### 3.2 Sol2 / OTA-V3 message (inside the serial transport data) [seen]+[code: o60, x00]

```
msg_id u16le | length u16le | payload[length]          (all integers little-endian)
```
Responses start their payload with `result u16le` (0 = OK). The app treats a non-zero result as
"Device Response Error Code" (`b10`).

## 4. Message catalogue

Ids are decimal (hex). Requests are phone to remote; responses are remote to phone (notify).

| Id | Name (from app strings) | Payload layout | Seen values |
|---|---|---|---|
| 40 (0x28) | VersionReq | `u16 4` (constant), `u16 otaV3Type` | `04 00 0a 00` |
| 41 (0x29) | VersionRsp | `u16 result, u16 deviceOtaVersion, u16 needReboot(1=yes)` | `0000 0400 0100` |
| 42 (0x2A) | RebootCmd | `u8 needReboot` (echo of rsp) | `01`. The remote then reboots into the OTA manager. |
| 43 (0x2B) | FileInitReq (30 B) | `u16 4, u16 otaV3Type, u32 fileCount, u32 totalBytes, u8 md5[16], u8 2, u8 1` | count=6 (5 images + ctrl), total=3,335,684 (sum of the 5 image files; ctrl not counted), md5 all zero |
| 44 (0x2C) | FileInitRsp (>=22 B) | `u16 result, u16 otaVersion, u8 resRspFreq, u8 binRspFreq, u16 resumeState, u32 resumeFileCount, u16 fsBlock, u16 resSliceLength, u16 binSliceLength` | `0, 1, 1, 20, 0, 0, 4096, 548, 0` |
| 45 (0x2D) | FileInitCompletedReq (8 B) | `u8 resume, u8 resFileWithPath, u32 fsBlock?, u16 0` | all zero |
| 46 (0x2E) | FileInitCompletedRsp | `u16 result, u16 ?, u32 ?` | zeros |
| 47 (0x2F) | FileStart, ctrl form | `u16 fileType = 2`, then **the whole ctrl_packet.bin** | 1655 B payload |
| 47 (0x2F) | FileStart, image form (17 B) | `u16 fileType = 3, u32 fileLength, u32 sliceCount, u16 imageId, u8 offset[4] (LE u32 from a hex string, 0 here), u8 resume` | see section 5 |
| 48 (0x30) | FileStartRsp | after ctrl: `u16 result, u8 isResume, u8 rspFreq, u32 completedCount, u16 fileIndex` (12 B). After an image: `u16 result` only. | ctrl: `0,0,20,0,0` |
| 49 (0x31) | FilePacket | `u32 sliceIndex (1-based, per file), u16 imageId, u16 sliceLen, u8 fileType = 3, data[sliceLen]` | sliceLen 548 except the last slice of each file |
| 50 (0x32) | FilePacketRsp (ack) | `u16 result, u16 newRspFreq?, u32 completedCount?` | always `0000 0000 00000000` |
| 51 (0x33) | FileEnd | `u8 moreFiles (1 = more follow), u8 fileType, u16 fileIndex` | ctrl `01 02 0000`; images `01 03 nnnn`; last `00 03 0400` |
| 52 (0x34) | FileEndRsp | `u16 result` | 0 |
| 53 (0x35) | FileTotalEnd | empty | |
| 54 (0x36) | FileTotalEndRsp | `u16 result` | 0 |
| 35 (0x23) | LinkLoseCheckReq (remote to phone) | `u16 result, u16 rspFreq, u32 completedCount` | not seen. The app resumes from completedCount. |
| 36 (0x24) | LinkLoseCheckRsp (phone to remote) | `u16 0` | not seen |
| 55 (0x37) | Abort (remote to phone) | `u16 result, u16 reason` | not seen. The app special-cases "Device abort:80" and retries after 3 s. |
| 57 (0x39) / 58 (0x3A) / 60 (0x3C) | Cancel rsp / LoseCheckFinish rsp / PreviewVideoError | `u16 result, u16 x` | not seen |

`otaV3Type` is 10 unless the decrypted ctrl packet has `dfu_ID == 2`, in which case it is 11
(`n10.OooO0OO` "getFirmwareOtaType"). Here it is 10. **[code]**

Field names for 44/45/46/48/50 come from the app's log strings ("otaVersion, resResponseFrequency,
binResponseFrequency, resumeState, resumeFileCount, fsBlock, resSliceLength, binSliceLength";
"fileIndex, completedCount, ResponseFrequency"). The byte offsets come from the parser in `x00`.
Exact names are **[code]**; meanings of the always-zero fields are **[guess]**.

## 5. Full sequence (handle 69)

| # | Time (UTC) | Dir | Message | Notes |
|---|---|---|---|---|
| 1 | 16:55:03.235 | | Connect (normal firmware). Full GATT discovery, which shows the sifli_sd **and** sifliapp services. | |
| 2 | 16:55:07.425 | S | Write Request CCCD 0x0013 = 0x0001 | |
| 3 | 16:55:07.663 | S/R | ATT MTU 517 / 517 | |
| 4 | 16:55:08.723 | S | 40 VersionReq `[4, 10]` | |
| 5 | 16:55:08.786 | R | 41 VersionRsp `result 0, otaVer 4, needReboot 1` | |
| 6 | 16:55:08.802 | S | 42 RebootCmd `[1]` | |
| 7 | 16:55:13.828 | | Link lost, reason 0x08 (supervision timeout). The remote rebooted. | about 5 s |
| 8 | 16:55:15.935 | | Reconnect to the same address. Discovery now shows **only GAP, GATT and sifli_sd** (OTA manager firmware). Name "X2R_xxxx". | |
| 9 | 16:55:20.174 / .446 | S | CCCD 0x0013 = 1, then MTU 517 | |
| 10 | 16:55:21.502 | S | 43 FileInitReq | |
| 11 | 16:55:21.567 | R | 44 FileInitRsp (sliceLen 548, binRspFreq 20) | |
| 12 | 16:55:21.584 / .626 | S/R | 45 / 46 InitCompleted | |
| 13 | 16:55:21.644 | S | 47 FileStart ctrl (`02 00` + ctrl_packet.bin, 7 fragments) | |
| 14 | 16:55:21.747 | R | 48 `result 0, isResume 0, rspFreq 20, completed 0` | |
| 15 | 16:55:21.765 / .806 | S/R | 51 FileEnd `01 02 0000` / 52 OK | |
| 16 | 16:55:21.821 | S | 47 FileStart img 0 outcom_app.bin len 1,323,340, 2415 slices | |
| 17 | 16:55:21.973 | R | 48 OK | The remote also asks for 15 ms params here. |
| 18 | 16:55:21.990-16:56:34 | S/R | 2415 x 49, a 50 ack after every 20 slices and after the last one | 72.7 s |
| 19 | 16:56:34.589 / 35.041 | S/R | 51 `01 03 0000` / 52 OK | about 450 ms (verify) |
| 20 | 16:56:35.054-16:56:37.6 | | img 1 outcom_lcpu.bin 44,196 B, 81 slices; FileEnd `01 03 0100` | |
| 21 | 16:56:37.627-16:56:38.6 | | img 2 outcom_lcpu_patch.bin 12,236 B, 23 slices; FileEnd `01 03 0200` | |
| 22 | 16:56:38.572 | S | img 3 outres.bin 791,748 B, 1445 slices. The FileStartRsp takes **2.5 s** (erase). | |
| 23 | 16:57:22.317 | S | FileEnd `01 03 0300` | |
| 24 | 16:57:22.574 | S | img 4 outfont.bin 1,164,164 B, 2125 slices. The FileStartRsp takes **3.6 s**. | |
| 25 | 16:58:33.740 / 34.190 | S/R | 51 FileEnd `00 03 0400` (last) / 52 OK | |
| 26 | 16:58:34.201 / .281 | S/R | 53 FileTotalEnd / 54 OK | |
| 27 | 16:58:38.506 | S | The phone disconnects (HCI Disconnect, reason 0x13), about 4.2 s later. The remote installs and reboots. | |
| 28 | 16:59:20.5 | | The remote advertises again as X2R_xxxx (normal firmware). | |

**File order:** ctrl_packet first, then images sorted by imageId: 0 outcom_app, 1 outcom_lcpu,
2 outcom_lcpu_patch, 3 outres, 4 outfont. The app builds the list as
`zu(2,ctrl,-1), zu(3,outres,3), zu(3,app,0), zu(3,lcpu,1), zu(3,patch,2), zu(3,font,4)` and
sorts it by imageId (`Collections.sort(..., rl(4))`). The server URL string is 6 URLs separated
by `&&`, in the order ctrl, outres, outcom_app, outcom_lcpu, outcom_lcpu_patch, outfont.
**[seen]+[code]**

**Bytes on the wire are the files, unchanged.** The ctrl payload equals ctrl_packet.bin (1653 B).
Concatenating each file's 49-payloads gives exactly the v9 file: all 5 match byte for byte, and
the indices run 1..N with no gaps or retransmits. There is no extra
encryption, compression or CRC added on the wire. **[seen]**

### 5.1 Inside the image files (already pre-sliced by SiFli's packer) [seen, verified for all v9 slices]

Each file is a sequence of 548-byte slices (the last one is shorter):
```
sha256(data) [32] | offset u32le (= k*512) | data [<=512]
```
All 6,089 slices pass the SHA-256 check and the offset check. So the remote can verify each slice.
The app does not compute anything; it just cuts the file every `resSliceLength` (548) bytes.
`sliceCount = ceil(fileLen/548)`.

### 5.2 ctrl_packet.bin [seen+code]

Layout (SDK `dfu_protocol.h`): `sha256 [32] | AES-256-CTR(content) | RSA-2048 PKCS#1 v1.5 signature [256]`.
- The app decrypts the content (`ll0.OooO0OO`, AES/CTR). The key is SiFli's **public default
  key** `tools/secureboot/sifli02/s01.bin`. The IV is the first 8 bytes of
  `sifli02/sig_hash.bin` followed by 8 zero bytes (the same construction as the SDK's
  `dfu_get_counter`). `sha256(plaintext)` equals the first 32 bytes, and the signature verifies
  with **SiFli's public sample key** `sifli02/sig_pub.der` (its private key `sig_pri.pem` is also
  published in the SDK). So the vendor ships with SiFli's development keys.
- Plaintext `dfu_control_packet_t`: `dfu_ID u8, HW_version u32, SDK_version u32, FW_version u32, FW_key[32], image_header_len u16, {blk_size u16, img_count u8, img_count x {sig[256], length u32, flag u16, img_id u8}}`.

| | v9 | v8 |
|---|---|---|
| dfu_ID | 1 (CODE_MIX) | 1 |
| HW_version | 4 | 0x33 (51, the SDK doc's placeholder value) |
| SDK_version | 0x15F96 (90006) | same |
| FW_version | **1001001** | **1001001** (the SDK doc's example value, so it does not change between releases) |
| blk_size / count | 512 / 5 | 512 / 5 |
| images (id: unpacked length, flag) | 0: 1,236,400 0x10; 1: 41,280 0x10; 2: 11,408 0x10; 3: 739,728 0; 4: 1,087,664 0 | 0: 1,227,856; 1: 41,184; 2: 11,408; 3: 609,072; 4: 1,087,664 |

flag 0x10 is DFU_FLAG_COMPRESS. No image has DFU_FLAG_ENC (the "out" prefix in the file names
means not encrypted, "com" means compressed). Each length equals the sum of the slice data in
that file.

## 6. Flow control and timing [seen]+[code]

- **Window of 20.** The response frequency (`binRspFreq` = 20 from 44, and `rspFreq` = 20 from
  48) sets the window. The app sends 20 FilePackets, then **waits for one 50 ack**, then sends
  the next 20. The final partial window of each file is also acked before FileEnd (counts seen:
  303 windows of 20, plus remainders 15, 1, 3, 5, 5). If the app sends more than rspFreq without
  an ack it fails ("continueSendNoResponsePacketCount OverRange", `z60`).
- Inside a window, slices go out back to back: each slice is 3 ATT writes, with a median of
  12.4 ms between slices (about one slice per 15 ms connection event). The median gap from an
  ack to the next slice is 11.8 ms. Per-file throughput is about 16-18 KB/s. The whole
  transfer took 193 s for 3.34 MB.
- The app inserts no deliberate sleeps beyond the connect-time 1 s and 200 ms waits.
  Command timeouts: 20 s for most requests and 60 s for some (`La10.OooO00o(20)`, `OooO0Oo=60`);
  "setCountDownMin180" applies during image transfer. **[code]**
- Slow points on the remote: the reboot into OTA mode (about 5 s until the link drops, then
  about 2 s to reconnect); FileStart of res/font (2.5-3.6 s, flash erase); FileEnd (0.07-0.45 s);
  the disconnect after TotalEnd (the app waits about 4 s).

## 7. Errors, resume, re-flash, safety

- **Errors** [code]: any non-zero `result`. The app handles device code 19 by resending the
  current file ("receive device code:19, sending current bin again"). It handles Abort (55) with
  reason 80 by retrying after 3 s. Some SDK error codes may apply (`dfu_err_type_t`: 7
  FW_VER_ERR, 9 CONTROL_PACKET_INVALID, 10 OTA_ONGOING, 15 MISS_PACKET, ...). The V3 numbering
  is **[guess]**.
- **Resume** [code]: FileInitRsp carries `resumeState` and `resumeFileCount`. The ctrl
  FileStartRsp carries `isResume, completedCount, fileIndex`. The image FileStart has `resume`
  and `offset` fields. LinkLoseCheckReq (35) lets the remote report `completedCount` after a
  hiccup. The app calls the library with `tryResume=false` (`Oooo000(Z)` gets false from the
  download callback), so resume is unused. In this log every resume field is 0.
- **Same version again (9 onto 9).** Probably accepted [guess, strong]. The phone does not ask
  the remote for its firmware version: VersionReq returns only the OTA protocol version (4) and
  a reboot flag. The ctrl packet's FW_version is the same (1001001) in v8 and v9, so the remote
  cannot tell releases apart from it. The SDK's `dfu_ctrl_compare_FW_version()` is a weak
  function that returns 1 (always allow) (`dfu_util.c`). The only SDK checks are dfu_ID,
  `SDK_version <= device`, OTA-in-progress and the image header. Version gating is done by the
  app and server (`software_code` POSTed to `remote2*` endpoints). `RemoteFirmwaresActivity`
  even lists release and beta builds to choose from.
- **Interrupted transfer** (SDK `dfu_service/firmware_flow.md` and `dfu_ctrl_sol.c`; it is
  assumed the closed V3 firmware follows the same design) [code/doc, guess for V3]:
  - The **OTA manager is a separate firmware** that boot runs first. That is why the reboot step
    exists and why OTA mode lacks the sifliapp service.
  - The **code images** (hcpu, lcpu, patch; compressed) go into a reserved download area. They
    are installed, meaning decompressed over the app, **only after the whole download and its
    checks pass**. If a code download fails, the remote returns to the old app.
  - **Resources and fonts are written in place.** If their download fails, the remote stays in
    the OTA manager until a later download succeeds. It remains reachable over BLE and the update
    can be resumed or restarted, so it does not brick.
  - It is not dual-bank for the app. Safety comes from download, verify, then install, done by
    the OTA manager.


## 9. Open questions

1. What the remote does when needReboot=0 (already in OTA mode, for example after an
   interrupted update). The app then skips the reboot (`b70` branch); the exact next step was
   not traced.
2. The meaning of the constant `u16 4` in 40/43, of the trailing `02 01` in 43, and of the
   zero fields in 45/46/50. They are copied as seen.
3. The full V3 error-code table and Abort reasons (80 is seen in the app only).
4. Whether the remote rejects a ctrl packet whose HW_version differs. v8 has 0x33 and v9 has 4,
   and the update was accepted on this unit. Whether it checks SDK_version against its own
   firmware.
5. How the Go side learns the remote's BLE address without the app's HubBean: from the hub
   protocol, a scan for `X2R_` plus a name match, or the hub-reported MAC.
6. Whether flashing 9 onto 9 is accepted (the analysis says yes). Testing that needs writing to
   the remote, which was not done.
7. The behaviour of `sifliapp` (0x14-0x1B) in normal mode is unknown. The OTA does not use it.
8. Device-side code for OTA V3 is closed. Section 7's safety model is inferred from the open SDK.
