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

## Next

1. **Activities.** Exported, applied and created from profiles, with
   their own sequences across devices; to confirm on the remote.
2. **Device sequences.** Sequences in device profiles ("3D on" on the
   projector), which `sbx` copies into every activity with the device and
   keeps in sync, since the hub only has activity sequences (PLAN R6).
3. **Bluetooth devices.** Read their commands (HID usages) into profiles,
   then create them: capture the app pairing a Bluetooth device (the UGOOS
   AM6B+, the Aetherion's Bluetooth remote) through the proxy, and work out
   the record's profile id (`code_id`). Upstream never managed it (PLAN L1,
   R1).
4. **Keys.** Unbinding a key.
5. **Restore.** Write a `dump` back to a hub.

## Later

- More IR protocols for profile codes (NECx, Sony, RC5/RC6, Samsung,
  Panasonic …) and import from open IR databases (PLAN P7, R3).
- Device category and icon (PLAN L5, R5).
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
- Whether the hub accepts device sequences beyond power on/off, and how
  the remote shows them.
- What the device category and icon change on the remote, beyond the
  picture.
