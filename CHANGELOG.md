# Changelog

All notable changes to this project are documented here. The format is based
on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added

- `sbx --help` starts with a banner: name, version and what sbx is.
- A profile for the Ugoos AM6B Plus running CoreELEC (NEC, from CoreELEC's
  remote configuration).

## [0.1.0] - 2026-10-31

The first prerelease: reading a hub, and creating and editing IR devices
from YAML profiles. Verified on an X2 hub (firmware 9).

### Added

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
