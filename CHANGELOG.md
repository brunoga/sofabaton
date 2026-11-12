# Changelog

All notable changes to this project are documented here. The format is based
on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added

- RF learning: `hub.Client.LearnRF` learns 433 and 315 MHz codes as the app
  does, `sbx learn -rf 433` adds one to a device, and profiles keep RF
  commands (`rf: 433` beside their `data`). Not yet tried with an RF remote.
- MQTT (X2): `hub.Client.SetMQTT`, `MQTTConfigured` and `ClearMQTT` set the
  hub's broker, and profiles make MQTT devices (`mqtt: true`), whose
  commands have the hub publish which ran; `sbx mqtt`. The hub's requests
  over MQTT, to list and run activities, devices and keys, are in
  docs/protocol/mqtt.md.
- Matter: `hub.Client.MatterCode` reads the X2's Matter setup code and
  `MatterDevice` the device of its switches; `sbx matter` shows the code as
  a QR code to add the hub to a Home app (docs/protocol/matter.md).
- IR learning: `hub.Client.Learn` has the hub learn a code from another
  remote, as the app does (family 0x04, then the code as a command
  write in family 0x06, a 0x68 for each attempt it could not read), and
  `sbx learn -device D -name N [-key K]` adds it to a device, bound to a
  key if asked. On our X2 it learned the receiver's Down on the first
  press, the Denon code its catalog profile has (2026-11-07).
- `sbx` checks every part of an activity from the remote: Roku commands
  reached a listener from its keys, macro, favorite, and power on and
  off, on the connection the hub keeps open.

### Fixed

- Applying a network device's profile that names a color no longer fails:
  its compact record holds none, so the color is left out, as its create
  does.
- Reading a device that has no commands (one made for learning, say) no
  longer fails: the hub answers its command list with status 0x07, which
  `profile.Read` takes as no commands when the device has no keys either.
  With keys, which run commands, it is the hub answering wrongly, as our
  X2 once did, and an error, to read again; `hub.Client.Commands` reports
  it as `hub.ErrNotFound`.

- When a new Bluetooth device does not pair, sbx says to create it again
  (the hub dropped it) instead of to `sbx pair` it, and to restart the hub
  if the device reports a wrong PIN: on our X2 that cleared what failed
  attempts left. A device we created paired and worked (2026-11-06).
- Applying an activity reads the commands and inputs of the devices it
  refers to only, not of every device, and once instead of twice when
  creating it; a device left as in `ApplyOptions.Base` whose activities
  changed is read once, to write its macros' copies. On our X2, adding
  an activity took 11 s instead of 31.

- The simulated hub has no members for a new activity until its
  POWER_ON macro is written, as our X2 does: it answered the members
  confirmed before the create.

- `ApplyOptions.Base` compares profiles as written: a config copied
  through YAML, as an editor's, lost a hidden field, and every device and
  activity counted as edited and was read again.
- The simulated hub gives a new device or activity the lowest free id,
  as our X2 does (it gave id 1, freed by a delete), not the highest plus
  one.

- A dump stops at the first failure that is not the hub refusing a
  request (a lost connection, a hub that stopped answering), and
  `dump.ReadProgress` and `sbx dump` report it. It used to carry on,
  recording every request after it as failed, and hand back a dump
  missing most of the hub: an app whose phone slept mid-read showed an
  empty hub.

### Added

- The catalog's key can be built into sbx, scrambled (`irsource.ScrambleKey`,
  `tools/scramblekey`, the linker's `-X main.builtinKey`): releases and CI
  take it from the repository's `SOFABATON_KEY` secret, `make` from
  `~/sofabaton-catalog-key.txt` if there is one. `-key` and
  `$SBX_SOFABATON_KEY` come first. It is never in the repository; the
  scrambling keeps it from a casual look at the binary, not from a search.
- X2 remote firmware updates: the `sifli` package speaks SiFli's update
  protocol, as the Sofabaton app updates the remote from the phone over
  Bluetooth (checked message by message against the phone's log of the
  app's update of our remote; docs/protocol/remote-firmware-update.md);
  `hub.Client.Remotes` reads the remotes the hub knows (firmware and
  Bluetooth address), `irsource` lists and downloads the remote's
  firmware, and `sbx firmware -remote` updates it over BlueZ on Linux.
  Armrest, on Android, sent our remote its firmware this way
  (2026-11-06).
- Hub firmware updates: `hub.Client.UpdateFirmware` sends an image as the
  app does (its size and MD5, then numbered chunks, each acknowledged);
  `irsource.SofabatonDB.HubFirmware` asks Sofabaton's server for a newer
  firmware, as the app does, and `HubImage` downloads it; `sbx firmware`
  says whether there is one and installs it with `-update`, then waits for
  the hub to come back with it. Our start frame for the 8 to 9 image is
  the app's; no newer hub firmware has been out to send yet.
- Philips Hue bridges: a profile's `hue` block (the bridge's address and
  user) and `hue` commands (a path and a JSON body the hub PUTs to the
  bridge), read from the hub and created as the app creates them
  (`record.HueRequest`, checked against the app's 100 commands). The `hue`
  package finds bridges (mDNS), gets a user with the link button, lists
  rooms and lights and makes the app's on, off and brightness commands;
  `sbx hue` writes the profile. A bridge device we made switched our
  lights on the X2 (2026-11-06).
- `dump.RefreshLists` reads only the hub's lists and returns the devices
  and activities whose row is new or changed, to read once shown: a new
  device of 101 commands took 15-20 s to show on our X2, the lists take
  about one. A dump's device not read yet is served with no commands.
- Bluetooth devices: a profile's `bluetooth` block (the catalog profile
  it came from, and whether it pairs over classic Bluetooth) makes a
  device whose commands are HID usages, and `profile.Create` (`sbx apply
  -create`) makes it as the app did on our X2, frame for frame but for
  the command ids, then has the hub pair (`hub.Client.Pair`, `sbx
  pair`). The hub keeps a new Bluetooth device only once it has paired:
  `ApplyOptions.Pair` pairs the ones `ApplyConfig` creates, which skips
  them without it. Exports carry the block; `irfile.SofabatonBluetooth`
  reads it from a catalog profile, and `irsource.Entry.Classic` its
  mode. The simulated hub pairs at once and lists a Bluetooth device
  only once paired.

- The display order of the devices and of the activities: `Snapshot`
  lists them in it, and `ApplyConfig` writes a config's order when it
  holds every device (or activity) of the hub, then has the X2 sync the
  order to the remotes (`hub.SortDevices`, `hub.SortActivities`,
  `hub.SyncOrder`, `record.EntitySortPages`). `sbx order` shows the
  order and moves the devices or activities named to the front. Checked
  on our X2 against the app's write, and live. The simulated hub keeps
  the order, puts a new device last and keeps the Sort through updates,
  as our X2 does.

- Network devices: a profile's commands can be HTTP requests (`http`:
  method, URL, headers, content type, body), and `profile.Create` (`sbx
  apply -create`) makes a device of them, a generic network device, in
  upstream's order. On our X2 its commands, a key bound to one and its
  power macro had the hub send their requests. `record.HTTPRequest`
  reads and writes the hub's form of them, and `profile.FromHub` gives
  them back as `http`.
- Sofabaton's Bluetooth profiles: `irsource.SofabatonDB.Bluetooth` lists
  the catalog's 27, and `irfile.ReadSofabaton` reads their keys, HID
  usages, as the hub's data (`Signal.Data`). The AWOL profile's 24 keys
  are byte for byte what our X2 holds for the projector made of it.
  Creating Bluetooth devices waits on knowing how the hub pairs.
- `record.DeviceTail.Compact`: the tail such a device is left with,
  without the address's bytes, which was misread.

- Raw IR: `record.RawIR` encodes and parses the timings the X2 learns
  and sends (verified on our X2: a learned code written back turned the
  projector on). Profiles add and create commands of raw IR data, and
  restoring creates devices of learned codes; only data that is not IR
  (Bluetooth) is still refused.
- `irgen`: RC5 and RC5X, RCA, Pioneer, NEC42, NEC with 16-bit commands and
  Kaseikyo of any vendor, rendered as raw timings.
- `irfile`: Flipper's raw signals and the protocols above become
  commands instead of being left out; Pronto codes (learned form, `0000`)
  are read too, one a line after their names.
- `irfile`: Home Assistant SmartIR's `.json` files (Broadlink's codes in
  base64 or hex, and Pronto), LIRC's `lircd.conf` (raw codes, and
  pulse-distance and RC5 remotes rendered from their timings), Girr
  (IrScrutinizer's XML: protocols, timings or Pronto), and, one a line,
  Global Caché's `sendir`, Broadlink's codes and Unfolded Circle's hex
  codes (checked against upstream's vectors).
- Export of a device's IR codes for other tools: `irfile.Signals` and
  `Format.Write`, `sbx export-ir`. Flipper Zero (codes in Flipper's
  protocols as parsed signals, the rest raw), irdb and Girr (protocols'
  codes; Girr adds every signal's timings), SmartIR (Broadlink),
  LIRC (raw codes), and, one a line, Pronto, Global Caché's `sendir`,
  Broadlink's and Unfolded Circle's codes. Each format reads back to the
  same signals.
- `irgen`: the protocols the X2 has descriptors for (NEC, NECx, Sony,
  JVC, RC6, Samsung20, Panasonic, Denon, Denon-K) rendered as timings, for
  formats of timings; NEC, NECx, Sony, RC6 and Panasonic match the
  Unfolded Circle vectors' timings.
- `irfile`: an Unfolded Circle Kaseikyo code of Denon's vendor is read
  as Denon-K.
- `irfile`: Unfolded Circle's Sharp codes (protocol 14): Denon's (their
  extension bits 0) become Denon codes, and Denon codes are written as
  them; others become timings (`irgen.Sharp`, its bits as the vectors').
  LIRC's RC6 remotes are rendered as lircd sends them (codes inverted,
  the toggle twice as long where `rc6_mask` or `toggle_bit` says).
- Templates for the Denon AVR-X8500H's Zone 2 and Zone 3, from Denon's IR
  chart (Denon-K subdevices 3 and 5; Zone 3's power, volume and older
  inputs are the chart's 15-bit codes). Every code renders to the
  chart's own Pronto code; not yet tested on a receiver.
- `category:` in device profiles: the device's kind in the app (the
  record's type byte), by name (TV, Blu-ray, Projector, Receiver: those
  the X2 app was seen to write) or number. `export` reads it, `apply`
  changes it, a create sets it (a TV when left out, as before). The names
  are the device types of Kookong's IR database, which the app uses (its
  `com.hzy.tvmao.ir.Device`: Set-top box 1, TV 2, Media player 3,
  Blu-ray 4, Projector 6, Receiver 7…), and the app's own 15 (Bluetooth)
  and 16 (Wi-Fi). The receiver, projector and media player templates
  have theirs.
- `irsource.SofabatonDB`: Sofabaton's own catalog of IR code sets (the
  one its app searches: 7,000 brands, models by number, each a set of X2
  descriptors), reached with the key its app encrypts with, which callers
  supply: it is not in the library. `irfile.ReadSofabaton` reads its code
  sets; `irfile.Signal.Descriptor` carries a descriptor in no protocol
  profiles render. `irsource.Search` also matches a model pattern
  (Sofabaton's model numbers), ranking a model named before one covered
  by a pattern.
- `irfile`: a Sofabaton code set's learned codes (timings in µs, `pulse`,
  at the carrier `fre`) are raw IR instead of being left out, and
  `irfile.SofabatonPower` names the commands its power data says switch
  the device on and off (discrete keys, or its toggle).
- `irgen`'s Denon and Denon-K match the 394 codes of the chart's main
  zone, Zone 2 and Zone 3 sheets (five kept as test data).

- `dump.RefreshSome`: a refresh that reads only the devices and
  activities known to have changed (as after writing to them), keeping
  the others from the old dump.

- `dump.RefreshRows`: a refresh that reads only the devices and
  activities whose row in the hub's lists is new or changed (a rename, a
  new icon), as an app connecting with a cached copy does; their
  commands, keys and macros are for it to read before showing them, as
  the official app does each time a screen opens. `dump.RefreshOnly`
  reads those without reading the lists again.

- `session.Session.Watch`: see the frames of a session already open,
  until stopped, to record what an operation exchanged.

- `profile.ApplyOptions.Base`: the configuration as read, so
  `ApplyConfig` leaves the devices and activities that were not edited
  alone instead of reading each from the hub first. On our X2 those reads
  took 86 s before any write, whatever the edit.

- `irsource`: irdb's and Flipper-IRDB's indexes of device files, to
  search, and the files, to read with `irfile` (irdb through jsDelivr,
  as it asks; Flipper-IRDB's list from GitHub, less its files converted
  from irdb). `irsource.Credit` is the notice each asks for.
- `irfile`: Flipper Zero's `.ir` files and irdb's `.csv` files of IR
  codes, read into device profiles. Codes in the protocols profiles
  render become commands, named the usual way ("Volume Up" for Vol_up
  or KEY_VOLUMEUP); raw timings and other protocols are left out and
  listed with why. `sbx import-ir -f file` writes the profile.
  Flipper's Kaseikyo signals of Panasonic's vendor become Panasonic
  codes, and of Denon's Denon-K codes (Flipper's Denon receiver files
  give the codes our X2's receiver has, 368 for volume up).
- More IR protocols in profiles, in irdb's device, subdevice and
  function: NECx (and Samsung32 as NECx), Sony12, Sony15, Sony20, JVC,
  RC6, Samsung20 and Panasonic, in the descriptor forms upstream found
  the X2 sends exactly. Not verified on our X2 yet.
- `profile.Icon.Path`: the X2's drawing of each icon, as an SVG path on
  a 24x24 view box (upstream's, MIT), and `profile.Color.RGB`: each
  palette color as the X2 app draws it. For apps that show devices and
  activities as the remote does.
- Clearing a device's key: `hub.Client.UnbindKey` takes devices as well
  as activities (the frame the app sends, captured on our X2:
  `0x10 [device, key]`, the one that deletes a command when given a
  command's id), and `profile.Plan` with prune clears the keys the
  profile does not bind, before removing commands. `hub.NextCommandID`
  keeps command ids below the keys' codes (`record.FirstKey`), so the
  two cannot be confused.

- `dump.Refresh` reads a hub again, keeping an older dump's signal for
  each command whose record is unchanged: signals are most of a dump's
  time. The hub keeps a command's record when its signal is rewritten
  under the same label, so the older dump must hold only signals known
  to be current (see its doc).

- Reads the hub leaves unanswered are asked again, up to
  `hub.ReadRetries` times: our X2 drops about one command read in a few
  hundred, which failed a whole-hub read now and then.

- A whole hub as one configuration, for apps: `profile.Snapshot` reads
  every device and activity as profiles (with progress), and
  `profile.ApplyConfig` makes a hub hold a `profile.Config`, matching by
  id (an edited snapshot: renames are renames) or by name (a restore),
  creating what is missing and, with `Delete`, deleting what the config
  lacks (activities first, devices last). `Config.RenameDevice`,
  `RenameCommand`, `RenameMacro`, `RemoveDevice`, `RemoveCommand`, `RemoveMacro` and `RemoveActivity` edit a config
  with every reference kept consistent. `dump.ReadProgress` reports
  progress. Long operations (`Apply`, `ApplyActivity`, `ApplyConfig`,
  `Snapshot`, `ReadProgress`) can be cancelled between steps.
  `profile.Restore` is now `Snapshot` then `ApplyConfig`, and no longer
  unbinds and rebinds keys that run device macros whose copies exist.

- `sbx restore -f dump.json`: makes a hub hold what a dump holds.
  Devices and activities are matched by name, applied, and created when
  missing (IR devices of codes and descriptors; activities, then their
  devices' macros, then the keys that run them); `-n` shows the plan,
  `-prune` as for `apply`. `profile.Restore` restores from any hub, and
  package `dump` serves a dump as a simulated hub. Checked on our X2: a
  device and an activity deleted and restored export as before.
- `sbx dump` keeps each command's signal and the remote's lists (a
  device's commands, an activity's macros and favorites), which restore
  needs; older dumps are refused by restore.

- `apply -prune` on an activity unbinds the keys its profile lacks
  (`0x10 [activity, key]` and the sync, upstream's flow; checked on our
  X2). A device's keys are left alone: there the frame deletes a
  command.

- Activity favorites: `favorites:` in an activity's profile, commands
  of its devices on the remote's list after its macros. `apply` adds
  them, reorders them and, with `-prune`, removes them; `export` reads
  them in their order, and `sbx list buttons` shows them. Checked on our
  X2, after upstream's flow: a binding write on slot 0 whose ack carries
  the favorite's id, the list (`0x61`) and the sync.

- `icon:` and `color:` in device and activity profiles: `export` reads
  them, and `apply` rewrites the record (and syncs) when they differ.
  They take the X2's names (`icon: Projector`, `color: green`, from
  upstream's tables) or the hub's numbers, and `export` writes names.

- Device macros: `macros:` in a device's profile, of its own commands and
  pauses. The remote cannot show a device's own macros (it shows each as
  the same blank entry), so `apply` copies them into every activity with
  the device and keeps the copies in step (`-prune` removes the ones the
  profile lacks). Each copy carries its device in its label slot: an
  activity's keys run it as `'Device: Macro'`, an activity's profile
  leaves it alone, an activity the device joins gets the device's macros
  and one it leaves loses them, and `export -device` reads them back.

- Device templates: read-only profiles of the devices we maintain, in
  `templates/` by transport and maker (`ir/denon/avr-x8500h`), carried by
  `sbx`. `sbx templates` lists them; `sbx new -template NAME -name NAME`
  makes a profile of one (its comments kept, `from:` naming the template)
  to edit and apply. `apply` refuses a template. `make check` runs the
  quick pre-push checks.

- `sbx apply` takes activity profiles: members and their inputs, power
  macros, the activity's own macros across its devices, keys and
  the name; `-create` makes a new activity, and `sbx remove-activity`
  deletes one (`profile.PlanActivity`, `ApplyActivity`,
  `CreateActivity`).
- `sbx list` takes its flags after its arguments too.
- `inputs: []` in a device profile: the device switches no inputs, as
  the app's "no input needed"; `export` writes it.
- Activity writes in `hub`: `ConfirmMember`, `CreateActivity`,
  `UpdateActivity`, `DeleteActivity`, `CreateMacro` (the hub assigns the
  id) and `DeleteMacro`; `record.NewActivity`. All but `DeleteMacro`
  rebuild the X2 app's frames byte for byte from our capture.
- `sbx export -activity`: an activity's profile (devices and their
  inputs, keys, power macros and macros across its devices), and
  `profile.Activity`, `ReadActivity` and `ActivityFromHub`.
- `sbx --help` starts with a banner: name, version and what sbx is.
- `hub.ErrUnsupported` and `Client.Writable`: the client refuses to write
  to an X1 or X1S, whose writes we have not tested; reads work. `sbx`
  says so before asking to apply.
- A template for the Ugoos AM6B Plus running CoreELEC (NEC, from CoreELEC's
  remote configuration).

### Changed

- `profiles/` is now `templates/`, its files templates: `template:` names
  the model, and the maintainer's hub ids and device names are gone (`sbx
  new` sets the name).

### Fixed

- Discovery skips an `sbx proxy` running on the same machine, which
  answers as the hub it holds.
- `sbx export` works on X1 and X1S hubs again: it only reads.

## [0.1.0] - 2026-10-31

The first prerelease: reading a hub, and creating and editing IR devices
from YAML profiles. Verified on an X2 hub (firmware 9).

### Added

- Activity favorites: `favorites:` in an activity's profile, commands
  of its devices on the remote's list after its macros. `apply` adds
  them, reorders them and, with `-prune`, removes them; `export` reads
  them in their order, and `sbx list buttons` shows them. Checked on our
  X2, after upstream's flow: a binding write on slot 0 whose ack carries
  the favorite's id, the list (`0x61`) and the sync.

- `sbx`, the command-line tool:
  - `discover`, `info`, `list` (devices, activities, a device's commands,
    keys, sequences) and `dump` (the whole configuration as JSON, raw
    bytes kept: a lossless backup).
  - `export` and `apply`: a device as a YAML profile, and a diffing apply
    that changes only what differs (`-n` shows the plan, `-prune` removes
    commands the profile lacks, `-create` makes a new IR device).
  - `add-ir`, `remove-ir`, `send`, `remove-device` and `sync`.
  - `proxy`: stands between the official app and the hub, relaying and
    recording every frame (`-capture`), with notes typed while it runs.
  - `-hub` is optional everywhere: it defaults to `$SBX_HUB`, else to the
    only hub that answers discovery. Devices are given by name or id.
- Package `profile`: YAML device profiles with commands in the remote's
  order, keys (short and long presses), power on/off commands and inputs;
  plans against a hub's state, their application, and creating a device
  from scratch in the app's order. Renaming a device or command keeps the
  activities and keys that use it. IR codes as NEC, Denon-K or Denon
  (Sharp) codes, per device or per command, or the hub's descriptors.
- Profiles for the AWOL Vision Aetherion projector (from AWOL's IR list)
  and the Denon AVR-X8500H receiver (from Denon's IR command chart).
- Package `hub`: the client. Connects, identifies the hub and reads
  devices, activities and their members, command lists, keymaps, command
  order, sequences and inputs; writes commands, key bindings, command
  order, device records (create, update, delete), idle behavior,
  sequences and inputs; sends commands; syncs the remotes.
- Packages `wire` (frames, stream decoding, paging, labels, the per-model
  record dimensions), `record` (the hub's records, re-encoded
  byte-identically), `transport` (discovery and CALL_ME), `session`
  (serialized exchanges with typed acks and errors), `proxy`, `capture`
  (JSON lines) and `hubsim` (a simulated hub for tests);
  `internal/mdns`, a minimal mDNS browser and responder.
- Protocol docs with a confidence level per fact, including the X2 app's
  write flows learned from our own captures, and a roadmap.
- Test data: frames from upstream's tests, vectors from the upstream
  implementation (`make oracle`), our X2's dumps and the app captures
  behind each write flow.
- `make ci` runs every CI job locally; releases are built with
  GoReleaser for Linux, macOS and Windows.

[Unreleased]: https://github.com/brunoga/sofabaton/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/brunoga/sofabaton/releases/tag/v0.1.0
