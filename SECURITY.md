# Security Policy

## Reporting a vulnerability

Please **do not** open a public GitHub issue for security vulnerabilities.

Use [GitHub private vulnerability reporting](https://github.com/brunoga/sofabaton/security/advisories/new) to report the issue confidentially. You will receive an acknowledgement within 48 hours and a resolution timeline within 7 days.

Please include:

- A description of the vulnerability and its potential impact
- Steps to reproduce or a proof-of-concept
- Any suggested fixes if you have them

## Scope

This repository is a client, a proxy and a command-line tool for a hub on
the local network. The hub protocol has no authentication or encryption, so
anything on the same network can talk to the hub; this library does not
change that and does not claim to.

- **Decoders** read untrusted bytes (from the hub, from the official app
  through the proxy, from capture files). A malformed frame must at worst
  produce an error, never a crash. They are fuzzed; a reproducible panic,
  hang, or unbounded memory growth on any input is a bug we want to hear
  about.
- **The proxy** listens on the local network and relays between the
  official app and the hub. A way to make it relay to or from anything else,
  or to write to the hub while it reports the app as attached, is in scope.
