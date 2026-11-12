# MQTT

The X2 talks to an MQTT broker, one per hub, in two ways: it publishes
which command ran for the commands of its MQTT devices (the app's "HA
remote"), and it answers requests, from anything on the broker, to list
its activities, devices and keys and to run them. Sofabaton's Home
Assistant integration ([ha-sofabaton-hub](https://github.com/yomonpet/ha-sofabaton-hub))
uses the second.

Checked on our X2, firmware 9, against a broker of our own (2026-11-12),
`[X2✓]`, unless marked `[app]` (the app's code) or `[ha]` (the
integration's).

## Setting the broker `[X2✓]`

| Frame | What |
|---|---|
| `73` setup | sets the broker; the hub answers `74 [01]` and connects at once |
| `74` (empty) | asks whether the hub has a broker: `74 [01]` yes, `74 [00]` no |
| `75` (empty) | unbinds it: `75 [01]`; the hub disconnects |

The setup's payload, big-endian:

| Field | Bytes |
|---|---|
| length of what follows | 2 |
| the broker's IPv4 address (no names) | 4 |
| its port | 2 |
| the hub's MAC (its banner's id) | 6 |
| the user's length in UTF-16 units, then the units | 1 + 2n |
| the password's, the same | 1 + 2n |

The app requires a user and a password `[app]`. The hub connects with
MQTT 3.1.1, client id `sofabaton_` and its MAC in lower case, keep-alive
1000 s, a clean session and no will, and subscribes to the request topics
below. `hub.Client.SetMQTT`, `MQTTConfigured` and `ClearMQTT`; `sbx
mqtt`.

## MQTT devices `[X2✓]`

A device of class `0x20`, MQTT, whose commands carry two bytes of data:
the device's id and the command's. Running one (from a key, a macro, the
app, or `3F`) has the hub publish to `MAC/up` (the MAC in upper case),
QoS 1, not retained:

    80F1B2962C08/up {"device_id":6,"key_id":2}

An automation on the broker's side acts on it. The app makes the device
`[app]`, and `profile` does the same (`mqtt: true` and commands of names
only; on our X2, 2026-11-12):

| # | Frame |
|---|---|
| 1 | `74`: the hub must have a broker |
| 2 | `07` the record: class `20`, type 16, inputs mode 1, power mode 1, style 0, the IR emitter routing, Bluetooth mode 0 (the app names it "tmp" until step 4) |
| 3 | `0E` per command, ids from 1, type `20`, data `[device, id]` |
| 4 | `08` the record with its name, marker 1 |

## Requests `[X2✓]` `[ha]`

The hub subscribes to these topics, `MAC` in upper case. Payloads are
JSON; the hub answers on the topic named, QoS 1.

| Topic | Payload | Answer |
|---|---|---|
| `activity/MAC/list_request` | `{"data": "activity_list"}` | `activity/MAC/list`: `{"data":[{"activity_id":101,"state":"off","activity_name":"…"}],"activity_count":4}` |
| `device/MAC/list_request` | `{"data": "device_list"}` | `device/MAC/list`: `{"data":[{"device_id":2,"device_name":"…"}]}` |
| `activity/MAC/keys_request` | `{"data": {"activity_id": 101}}` | `activity/MAC/keys_list`: `{"data":[{"key_id":174}],"activity_id":101,"key_count":11}`, the keys bound |
| `device/MAC/keys_request` | `{"data": {"device_id": 6}}` | `device/MAC/keys_list`: `{"data":[{"key_id":1,"key_name":"Scene one"}],"device_id":6,"key_count":2}`, its commands |
| `activity/MAC/macro_keys_request` | `{"data": {"activity_id": A}}` | `activity/MAC/macro_keys_list` `[ha]` |
| `activity/MAC/favorites_keys_request` | `{"data": {"activity_id": A}}` | `activity/MAC/favorites_keys_list` `[ha]` |
| `device/MAC/keys_control` | `{"data": {"device_id": 6, "key_id": 1}}` | runs the command |
| `activity/MAC/keys_control` | `{"data": {"activity_id": A, "key_id": K}}` | presses a key of the activity `[ha]` |
| `activity/MAC/macro_keys_control` | `{"data": {"activity_id": A, "key_id": K}}` | runs a macro `[ha]` |
| `activity/MAC/favorites_keys_control` | `{"data": {"activity_id": D, "key_id": K}}` | runs a favorite; the integration sends the device's id as `activity_id` `[ha]` |
| `activity/MAC/activity_control_down` | `{"data": {"activity_id": A, "state": "on"}}` | starts (`on`) or ends (`off`) an activity `[ha]` |

The hub publishes `activity/MAC/activity_control_up` when an activity
starts or ends `[ha]`. Keys are the remote's (174 Up, 176 OK, 182
Volume Up, …: record.Button). The integration's authors note that the
hub can publish an activity's change before its state has changed, and
does not reliably send large answers.
