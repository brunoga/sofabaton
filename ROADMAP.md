# Roadmap

What works, what is next, and what is still unknown. [PLAN.md](PLAN.md) has
the design and the phases behind it; [docs/](docs) has the protocol.

## Works (0.1, verified on our X2)

- Finding hubs (broadcast discovery) and connecting (CALL_ME and the hub's
  connect-back).
- Reading the whole configuration: devices, activities and their members,
  commands, keymaps, sequences, inputs. `sbx dump` is a lossless backup.
- The official-app proxy with captures, our main research tool.
- IR devices from YAML profiles: export; a diffing apply (add, rename,
  re-code, remove and reorder commands, bind keys short and long, power
  sequences, inputs, renaming the device); creating a device from scratch;
  deleting one; syncing the remote.
- IR codes as NEC, NECx, Denon-K, Denon (Sharp), Sony, JVC, RC6,
  Samsung20 and Panasonic codes, or the hub's own descriptors.
- Device files of IR codes: Flipper Zero's `.ir` and irdb's `.csv` read
  into profiles (`irfile`, `sbx import-ir`), and irdb and Flipper-IRDB
  searched, browsed by type and maker, and fetched (`irsource`).
- Sending a device's command.
- Device macros: in device profiles, copied into every activity with the
  device (the remote cannot show a device's own), kept in step, and run
  from activity keys as "Device: Macro".
- Device templates, by transport and maker, carried by `sbx` (`sbx new`).
- Icons and colors of devices and activities, by name.
- Activity favorites: added, ordered and removed.
- The order of the devices and activities (`sbx order`, and in restores).
- Unbinding an activity's key (`apply -prune`).
- `sbx restore`: a dump written back to a hub, by name, creating what is
  missing (IR devices of codes, descriptors and learned codes, Bluetooth
  devices, paired again, network and Hue devices, activities).
- Bluetooth devices: created as the app creates them, from catalog
  profiles, and paired; one of ours paired with a phone and its keys
  worked (2026-11-06).
- Philips Hue bridges' rooms and lights as devices (`sbx hue`).
- Firmware: the hub's found and sent as the app does (`sbx firmware`), and
  the remote's over Bluetooth (`sbx firmware -remote`, Linux; checked from
  Armrest on Android, 2026-11-06).

## Next

1. **Activities on the remote.** Activities are exported, applied and
   created from profiles, with their own macros across devices, and read
   back from the hub as written (Armrest made, edited and deleted one on
   our X2, 2026-11-05); still to see one of ours started from the remote.

## Later

- What the device category changes (PLAN L5, R5).
- RF devices (PLAN L2, R2) and the other network devices (Roku, Sonos,
  MQTT; PLAN P9). Generic HTTP devices are made since 2026-11-04, Hue
  bridges since 2026-11-06.
- A web UI over the library (PLAN P10).

## Unknown or untested

- X1 and X1S hubs: modeled from upstream's captures, never run by us. The
  client reads them and refuses to write to them (`hub.ErrUnsupported`).
  Supporting them needs someone with one: proxy captures of the app and
  an `sbx dump`. Their IR is raw timings, not the X2's descriptors, so
  profile codes there also need our own IR encoders (PLAN P7).
- What the device category and icon change on the remote, beyond the
  picture.
