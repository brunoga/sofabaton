# Changelog

All notable changes to this project are documented here. The format is based
on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added

- Repository scaffold: module, `sbx` command stub, CI, release
  configuration, the port plan and the first protocol docs.
- `make ci` runs every CI job locally.
- Package `wire`: frames and their checksum, a resynchronizing stream
  decoder and an encoder, opcode families named per direction, fixed-width
  ASCII and UTF-16BE label slots, the paged-write wrapper, and the per-model
  record dimensions (X1, X1S, X2).
- Package `record`: the device/activity record (`Entity`) with its device
  tail and X2 emitter routing, and catalog rows. Decoded records re-encode
  byte-identically; edits rewrite only the changed fields.
- `testdata/captures/upstream.json`: 171 frames extracted from the upstream
  project's tests (125 complete), and `tools/extractcaptures`, which
  extracts them.
