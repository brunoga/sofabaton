# Captures

Hub frames used as test vectors.

## upstream.json

Frames embedded in the tests of
[m3tac0de/home-assistant-sofabaton-x1s](https://github.com/m3tac0de/home-assistant-sofabaton-x1s)
(MIT, see `NOTICE`), extracted statically: the Python sources are read as
text, never executed.

```sh
git clone https://github.com/m3tac0de/home-assistant-sofabaton-x1s /tmp/upstream
go run ./tools/extractcaptures -upstream /tmp/upstream \
    -commit "$(git -C /tmp/upstream rev-parse HEAD)" > testdata/captures/upstream.json
```

Each entry has the frame's hex, where it appears upstream (`path:line`),
the test or name it belongs to and the comment above it.

`complete` is true for the frames that are exactly one valid frame. The
others (46 of 171 at commit `5fd5887`) are fixtures that upstream trimmed,
padded or built from expressions; its parsers tolerate them. They are kept for
record-level tests, never for frame-level ones.

Many of these frames come from X1 and X1S hubs. Frames from our own X2 go in
separate files, one per captured flow, with a note on what was done in the
app while it was recorded (see `docs/research/README.md`).
