# Changelog

All notable changes to this project are documented here. The format is based
on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Fixed

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
