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
- IR codes as NEC, Denon-K and Denon (Sharp) codes, or the hub's own
  descriptors.
- Sending a device's command.
- Device macros: in device profiles, copied into every activity with the
  device (the remote cannot show a device's own), kept in step, and run
  from activity keys as "Device: Macro".
- Device templates, by transport and maker, carried by `sbx` (`sbx new`).
- Icons and colors of devices and activities, by the hub's numbers.

## Next

1. **Activities.** Exported, applied and created from profiles, with
   their own macros across devices; to confirm on the remote.
2. **Bluetooth devices.** Read their commands (HID usages) into profiles,
   then create them: capture the app pairing a Bluetooth device (the UGOOS
   AM6B+, the Aetherion's Bluetooth remote) through the proxy, and work out
   the record's profile id (`code_id`). The Aetherion's Bluetooth device,
   since deleted, is kept in `testdata/x2/dump-2026-11-01.json` and
   `bluetooth-projector.yaml`. Upstream never managed it (PLAN L1,
   R1).
3. **Keys.** Unbinding a key, and an activity's favorites: capture the
   app adding and removing one.
4. **Restore.** Write a `dump` back to a hub.

## Later

- More IR protocols for profile codes (NECx, Sony, RC5/RC6, Samsung,
  Panasonic …) and import from open IR databases (PLAN P7, R3).
- Device category (PLAN L5, R5), and names for the icon and color
  numbers.
- RF devices (PLAN L2, R2) and Wi-Fi commands (IP, Roku, Hue, Sonos, MQTT;
  PLAN P9).
- The Denon receiver's Zone 2 and Zone 3 codes.
- A web UI over the library (PLAN P10).

## Unknown or untested

- X1 and X1S hubs: modeled from upstream's captures, never run by us. The
  client reads them and refuses to write to them (`hub.ErrUnsupported`).
  Supporting them needs someone with one: proxy captures of the app and
  an `sbx dump`. Their IR is raw timings, not the X2's descriptors, so
  profile codes there also need our own IR encoders (PLAN P7).
- What the device category and icon change on the remote, beyond the
  picture.
