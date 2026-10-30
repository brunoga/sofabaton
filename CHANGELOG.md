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
- `wire.Assembler` joins the pages of a paged reply; `record.Command` and
  `record.ParseCommandList` decode command lists.
- `record.Binding` and `record.ParseKeymap`: keymap rows (family 0x3D) and
  binding writes (0x3E); `record.Button`, the remote's keys.
- `wire.Burst` collects replies of several paged records; `record.Macro`
  decodes macro records and builds their (paged) writes.
- `record.Inputs`: a device's inputs page (family 0x47 replies, 0x46
  writes).
- `record.HubInfo`: the hub identity of the banner and of the discovery
  reply; the discovery families (NOTIFY_ME, CONNECT_READY) in `wire`.
- Package `transport`: `Dialer` (CALL_ME and the hub's TCP connect-back),
  `Discoverer` (NOTIFY_ME broadcast discovery) and `Conn`, frames over a
  connection.
- `testdata/oracle`: vectors from the upstream implementation (labels,
  paging, 180 generated device records, upstream's decoding of the corpus's
  records, upstream's decoding of the corpus's command bursts, binding writes, macro writes and the corpus's macro bursts, inputs pages),
  `tools/oracle` and `make oracle`, which regenerate them.
- `testdata/captures/upstream.json`: 171 frames extracted from the upstream
  project's tests (125 complete), and `tools/extractcaptures`, which
  extracts them.
