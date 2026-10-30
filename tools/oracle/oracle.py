"""Generate test vectors from the upstream Python implementation.

The Go code must produce the same bytes as upstream for the same inputs
(or the difference must be a documented, deliberate one). This script
calls upstream's pure builders and parsers and writes their outputs to
testdata/oracle/*.json, which the Go tests read. It never talks to a hub.

    make oracle UPSTREAM=/path/to/home-assistant-sofabaton-x1s

Inputs are generated from a fixed seed, so a rerun at the same upstream
commit writes the same files.
"""

from __future__ import annotations

import argparse
import json
import random
import subprocess
import sys
from pathlib import Path

# The commit the vectors were generated at; also pinned in testdata/captures.
PINNED_COMMIT = "5fd5887d11272a553da99383f327c53cb8b4190e"

SEED = 20261030

# Text that exercises the label encoders: ASCII, Latin-1, CJK, emoji (a
# surrogate pair in UTF-16) at varying positions, and long strings that
# must be cut.
SAMPLE_TEXT = [
    "",
    "TV",
    "Bose",
    "Living room TV",
    "Télé du salon",
    "Fernseher Wohnzimmer Übersicht",
    "电视机",
    "📺",
    "TV 📺",
    "a" * 29 + "📺",
    "a" * 30,
    "a" * 31,
    "Ω" * 40,
    "Play Playstation 5",
]


def load_upstream(root: Path):
    sys.path.insert(0, str(root / "custom_components" / "sofabaton_x1s"))
    from lib import commands, device_create, devices, macros, wire_schema  # noqa: PLC0415

    return commands, device_create, devices, macros, wire_schema


def upstream_commit(root: Path) -> str:
    out = subprocess.run(
        ["git", "-C", str(root), "rev-parse", "HEAD"],
        check=True, capture_output=True, text=True,
    )
    return out.stdout.strip()


def labels(wire_schema) -> list[dict]:
    cases = []
    for enc in ("ascii", "utf-16-be"):
        for text in SAMPLE_TEXT:
            for n in (0, 1, 2, 3, 4, 20, 30, 40, 60, 61):
                cases.append({
                    "encoding": enc,
                    "text": text,
                    "len": n,
                    "slot": wire_schema.encode_label_slot(text, n, enc).hex(),
                })
    return cases


def paging(wire_schema) -> list[dict]:
    cases = []
    for n in (0, 1, 246, 247, 248, 493, 494, 495, 1000):
        body = bytes(i & 0xFF for i in range(n))
        cases.append({
            "body_len": n,
            "pages": [p.hex() for p in wire_schema.page_family_body(body)],
        })
    return cases


ENTITY_FIELDS = (
    "name", "brand", "device_id", "record_kind", "icon", "sort", "code_type",
    "device_type", "hide", "input_flag", "channel", "power_state",
    "ip_address", "poll_time", "input_mode", "power_mode", "power_style",
    "share_mode", "tail_flag", "tail_marker", "extra_a", "extra_b",
    "extra_c", "extras_present", "color_id", "bt_mode",
)


def config_json(cfg) -> dict:
    out = {f: getattr(cfg, f) for f in ENTITY_FIELDS}
    out["code_id"] = bytes(cfg.code_id).hex()
    return out


def random_config(devices, rng: random.Random):
    b = lambda: rng.randrange(256)  # noqa: E731
    return devices.DeviceConfig(
        name=rng.choice(SAMPLE_TEXT),
        brand=rng.choice(SAMPLE_TEXT),
        device_id=rng.choice((0xFF, 1, 0x0D, 0x63, 0x65, 0xC8)),
        record_kind=rng.choice((0, 0, 0, b())),
        icon=b(), sort=b(),
        code_type=rng.choice((0x03, 0x0A, 0x0D, 0x1A, 0x1B, 0x1C, 0x1D, 0x20)),
        device_type=b(),
        code_id=bytes(b() for _ in range(16)) if rng.random() < 0.5 else bytes(16),
        hide=b(), input_flag=b(), channel=b(), power_state=b(),
        ip_address=rng.choice((None, "192.168.1.128", "10.0.0.2", "0.0.0.0")),
        poll_time=rng.choice((-1, 0, 5, 0x1234)),
        input_mode=b(), power_mode=b(), power_style=b(), share_mode=b(),
        tail_flag=b(), tail_marker=rng.choice((0, 1)),
        extra_a=b(), extra_b=b(), extra_c=b(),
        extras_present=rng.random() < 0.5,
        color_id=rng.randrange(13), bt_mode=rng.choice((0, 2, 0xFF)),
    )


def entities(devices) -> list[dict]:
    rng = random.Random(SEED)
    cases = []
    for variant in ("X1", "X1S", "X2"):
        for _ in range(60):
            cfg = random_config(devices, rng)
            payload = devices.build_device_create_payload(cfg, hub_version=variant)
            parsed = devices.parse_device_record(payload[3:], hub_version=variant)
            cases.append({
                "variant": variant,
                "config": config_json(cfg),
                "payload": payload.hex(),
                "parsed": config_json(parsed),
            })
    return cases


def corpus_records(devices, corpus: Path) -> list[dict]:
    """Upstream's decoding of every complete record frame in our corpus."""
    caps = json.loads(corpus.read_text())["captures"]
    cases = []
    for c in caps:
        if not c["complete"]:
            continue
        raw = bytes.fromhex(c["hex"])
        if raw[3] not in (0x07, 0x08, 0x0B, 0x37, 0x3B):
            continue
        payload = raw[4:-1]
        variant = "X1" if len(payload) == 123 else "X1S"
        if len(payload) not in (123, 213):
            continue
        parsed = devices.parse_device_record(payload[3:], hub_version=variant)
        cases.append({
            "source": c["sources"][0],
            "variant": variant,
            "parsed": config_json(parsed),
        })
    return cases


def command_bursts(commands, corpus: Path) -> list[dict]:
    """Upstream's assembly and decoding of the corpus's command-list bursts.

    Complete family-0x5D frames are grouped by the test they come from, in
    source order, and split into bursts at each page 1."""
    caps = json.loads(corpus.read_text())["captures"]
    groups: dict[str, list[bytes]] = {}
    for c in caps:
        raw = bytes.fromhex(c["hex"])
        if not c["complete"] or len(raw) < 8 or raw[3] != 0x5D:
            continue
        path, line = c["sources"][0].rsplit(":", 1)
        groups.setdefault(f"{path}::{c.get('context', '').split('/')[0]}", []).append(
            (int(line), raw)
        )

    cases = []
    for key, frames in sorted(groups.items()):
        frames.sort(key=lambda f: f[0])
        bursts: list[list[bytes]] = []
        for _, raw in frames:
            page = int.from_bytes(raw[5:7], "big")
            if page == 1 or not bursts:
                bursts.append([])
            bursts[-1].append(raw)
        for burst in bursts:
            first = burst[0][4:-1]
            if int.from_bytes(first[1:3], "big") != 1:
                continue
            count = first[6]
            pages = int.from_bytes(first[4:6], "big")
            if len(burst) != pages:
                continue
            rest = len(first) - 7
            variant = "X1" if rest % 40 == 0 and rest % 70 != 0 else "X1S"
            assembler = commands.DeviceCommandAssembler()
            done = []
            for raw in burst:
                done.extend(assembler.feed(int.from_bytes(raw[2:4], "big"), raw, hub_version=variant))
            if len(done) != 1:
                continue
            dev_id, body = done[0]
            records = list(commands.iter_command_records_from_assembled(
                body, count=count, dev_id=dev_id, hub_version=variant,
            ))
            cases.append({
                "source": key,
                "variant": variant,
                "frames": [raw.hex() for raw in burst],
                "device": dev_id,
                "records": [{
                    "device": r.dev_id, "id": r.command_id, "control": bytes(r.control).hex(),
                    "label": r.label, "sort": r.sort_id,
                } for r in records],
            })
    return cases


def bindings(device_create) -> list[dict]:
    """Button-binding writes (family 0x3E) for generated bindings."""
    rng = random.Random(SEED + 1)
    cases = []
    for _ in range(100):
        args = {
            "device_id": rng.choice((0x01, 0x0D, 0x65, 0x66, 0xC8)),
            "button_id": rng.choice((0x01, 0x02, 0xAE, 0xB0, 0xB6, 0xC6, 0xC7, 0x97)),
            "short_press_device_id": rng.randrange(256),
            "short_press_button_code": rng.randrange(1 << 48),
            "short_press_button_id": rng.randrange(256),
        }
        if rng.random() < 0.5:
            args.update(
                long_press_device_id=rng.randrange(256),
                long_press_button_code=rng.randrange(1 << 48),
                long_press_button_id=rng.randrange(256),
            )
        step = device_create.build_button_binding_step(**args)
        cases.append({
            "args": args,
            "family": step.family,
            "payload": bytes(step.payload).hex(),
            "ack_opcode": step.ack_opcode,
        })
    return cases


def macro_writes(device_create) -> list[dict]:
    """Macro writes (family 0x12) for generated macros, including ones that
    need more than one page."""
    rng = random.Random(SEED + 2)
    cases = []
    for variant in ("X1", "X1S", "X2"):
        for steps in (0, 1, 3, 18, 19, 21, 22, 40):
            rows = b"".join(bytes(rng.randrange(256) for _ in range(10)) for _ in range(steps))
            label = rng.choice(SAMPLE_TEXT)
            key = rng.choice((0x01, 0x05, 0xC6, 0xC7))
            entity = rng.choice((0x65, 0x66, 0x0D))
            pages = device_create.build_macro_steps(
                hub_version=variant, device_id=entity, key_id=key,
                label=label, step_records=rows,
            )
            cases.append({
                "variant": variant, "entity": entity, "key": key, "label": label,
                "steps": rows.hex(),
                "pages": [bytes(p.payload).hex() for p in pages],
                "acks": [p.ack_opcode for p in pages],
            })
    return cases


def macro_bursts(macros, corpus: Path) -> list[dict]:
    """Upstream's assembly and decoding of the corpus's macro bursts."""
    caps = json.loads(corpus.read_text())["captures"]
    groups: dict[str, list] = {}
    for c in caps:
        raw = bytes.fromhex(c["hex"])
        if not c["complete"] or len(raw) < 8 or raw[3] != 0x13:
            continue
        path, line = c["sources"][0].rsplit(":", 1)
        groups.setdefault(f"{path}::{c.get('context', '').split('/')[0]}", []).append((int(line), raw))
    cases = []
    for key, frames in sorted(groups.items()):
        frames.sort(key=lambda f: f[0])
        first = frames[0][1][4:-1]
        body = first[3:]
        n = body[5]
        variant = "X1" if len(body) == 37 + 10 * n else "X1S"
        assembler = macros.MacroAssembler()
        done = []
        for _, raw in frames:
            done.extend(assembler.feed(int.from_bytes(raw[2:4], "big"), raw[4:-1], raw))
        if len(done) != 1:
            continue
        activity, payload, boundaries = done[0]
        records = macros.parse_macro_records_from_burst(
            payload, activity_id=activity, record_boundaries=boundaries, hub_version=variant,
        )
        cases.append({
            "source": key,
            "variant": variant,
            "frames": [raw.hex() for _, raw in frames],
            "records": [{
                "activity": r.activity_id, "key": r.key_id, "label": r.label,
                "slot": bytes(r.raw_label_slot).hex(),
                "steps": [[e.device_id, e.key_id, e.fid, e.duration, e.delay] for e in r.key_sequence],
            } for r in records],
        })
    return cases


def write(out_dir: Path, name: str, commit: str, cases) -> None:
    path = out_dir / f"{name}.json"
    doc = {
        "upstream": "github.com/m3tac0de/home-assistant-sofabaton-x1s",
        "commit": commit,
        "generator": "tools/oracle/oracle.py",
        "cases": cases,
    }
    path.write_text(json.dumps(doc, indent=1, ensure_ascii=False) + "\n")
    print(f"{path}: {len(cases)} cases")


def main() -> None:
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("--upstream", required=True, type=Path)
    ap.add_argument("--out", required=True, type=Path)
    ap.add_argument("--corpus", required=True, type=Path)
    ap.add_argument("--allow-other-commit", action="store_true")
    args = ap.parse_args()

    commit = upstream_commit(args.upstream)
    if commit != PINNED_COMMIT and not args.allow_other_commit:
        sys.exit(f"upstream is at {commit}, vectors are pinned to {PINNED_COMMIT}")

    commands, device_create, devices, macros, wire_schema = load_upstream(args.upstream)
    args.out.mkdir(parents=True, exist_ok=True)
    write(args.out, "labels", commit, labels(wire_schema))
    write(args.out, "paging", commit, paging(wire_schema))
    write(args.out, "entities", commit, entities(devices))
    write(args.out, "corpus_records", commit, corpus_records(devices, args.corpus))
    write(args.out, "commands", commit, command_bursts(commands, args.corpus))
    write(args.out, "bindings", commit, bindings(device_create))
    write(args.out, "macro_writes", commit, macro_writes(device_create))
    write(args.out, "macros", commit, macro_bursts(macros, args.corpus))


if __name__ == "__main__":
    main()
