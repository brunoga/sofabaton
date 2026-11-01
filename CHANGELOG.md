# Changelog

All notable changes to this project are documented here. The format is based
on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added

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
