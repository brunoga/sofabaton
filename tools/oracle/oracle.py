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
    from lib import devices, wire_schema  # noqa: PLC0415

    return devices, wire_schema


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

    devices, wire_schema = load_upstream(args.upstream)
    args.out.mkdir(parents=True, exist_ok=True)
    write(args.out, "labels", commit, labels(wire_schema))
    write(args.out, "paging", commit, paging(wire_schema))
    write(args.out, "entities", commit, entities(devices))
    write(args.out, "corpus_records", commit, corpus_records(devices, args.corpus))


if __name__ == "__main__":
    main()
