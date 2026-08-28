#!/usr/bin/env python3
"""Read a scenario, and judge one run of it against what the assistant said.

Three things are checked per run, and all three must hold:

  must_call        Did it reach the tools it needed? An answer written from the
                   model's own memory, with Margince never asked, is a failure
                   however good the prose reads.
  must_mention     Does the answer contain what it has to contain? Each entry is
                   a regex, and alternatives inside one entry are an any-of
                   group.
  must_not_mention Does it avoid what it must avoid? This half matters more than
                   the first: a confident wrong answer is worse than no answer.

Deliberately NOT judged: wording, tone, length, formatting, the order it did
things in, or extra correct information. Only whether the facts are right and
the required things were said.

Stdlib only — no PyYAML. The scenario files are a small fixed subset of YAML
(scalars, block strings, flat lists) and a dependency for that would make the
lane refuse to run on a fresh checkout.
"""

import json
import os
import re
import sys
import tempfile

# The two trees every path this script opens actually lives under: the repo
# itself (scenario files, and the records directory they get written to) and
# the system temp directory (the per-run transcript files scripts/e2e-llm.sh
# writes under its own `mktemp -d`). Neither is a fixed single directory —
# E2E_LLM_SCENARIOS/E2E_LLM_RECORDS can move the first, and the mktemp name is
# different on every run — so the check is containment in one of the two
# roots, not an exact match.
_REPO_ROOT = os.path.realpath(
    os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
)
_TEMP_ROOT = os.path.realpath(tempfile.gettempdir())


def _open_checked(path):
    """Open a file this script was told to read, refusing anything outside
    the repo or the system temp directory.

    Every path this script opens arrives as one of its own CLI arguments,
    built by scripts/e2e-llm.sh from a scenario glob or its own mktemp
    workdir — never from anything the assistant under test said or called, so
    this is not a defense against a hostile caller. It exists because those
    arguments are still just strings by the time they reach here, and a typo
    or a misconfigured E2E_LLM_SCENARIOS should fail with a clear reason
    rather than open whatever the resulting path happened to resolve to.
    """
    real = os.path.realpath(path)
    roots = (_REPO_ROOT + os.sep, _TEMP_ROOT + os.sep)
    if real != _REPO_ROOT and not real.startswith(roots):
        raise ValueError(f"refusing to open {path!r}: outside the repo and the system temp directory")
    return open(real, encoding="utf-8")


def parse_scenario(path):
    """Read the subset of YAML the scenario files use."""
    data, key, block, block_indent, seq = {}, None, None, 0, None
    for raw in _open_checked(path).read().splitlines():
        if block is not None:
            # A block scalar runs until a line that is neither blank nor
            # indented past the key.
            if raw.strip() == "" or raw.startswith(" " * block_indent):
                block.append(raw[block_indent:] if len(raw) >= block_indent else "")
                continue
            data[key] = "\n".join(block).strip()
            block = None

        line = raw.rstrip()
        if not line.strip() or line.lstrip().startswith("#"):
            continue

        if seq is not None and line.startswith("  - "):
            seq.append(_scalar(line[4:].strip()))
            continue
        seq = None

        m = re.match(r"^([A-Za-z_][A-Za-z0-9_]*):\s*(.*)$", line)
        if not m:
            continue
        key, value = m.group(1), m.group(2).strip()
        if value == "|":
            block, block_indent = [], 2
        elif value == "":
            seq = data[key] = []
        elif value.startswith("[") and value.endswith("]"):
            inner = value[1:-1].strip()
            data[key] = [_scalar(v.strip()) for v in inner.split(",")] if inner else []
        else:
            data[key] = _scalar(value)
    if block is not None:
        data[key] = "\n".join(block).strip()
    return data


def _scalar(text):
    if len(text) >= 2 and text[0] == text[-1] and text[0] in "\"'":
        text = text[1:-1]
    if re.fullmatch(r"-?\d+", text):
        return int(text)
    return text


def read_transcript(path):
    """Pull the tool names called and everything the assistant said.

    stream-json is JSONL. Tool calls arrive as `tool_use` content blocks and the
    prose as `text` blocks; the terminal `result` event carries the final answer
    and is included so a scenario checking the closing sentence sees it.
    """
    called, said = [], []
    for line in _open_checked(path):
        line = line.strip()
        if not line:
            continue
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            continue

        if event.get("type") == "result" and isinstance(event.get("result"), str):
            said.append(event["result"])

        message = event.get("message") or {}
        content = message.get("content")
        if not isinstance(content, list):
            continue
        for block in content:
            if not isinstance(block, dict):
                continue
            if block.get("type") == "tool_use":
                called.append(block.get("name", ""))
            elif block.get("type") == "text":
                said.append(block.get("text", ""))
    return called, "\n".join(said)


def tool_matches(called, want):
    """A tool is reached when any call names it.

    MCP tools arrive prefixed (mcp__margince__query_workspace), so the scenario
    names the bare tool and this matches the suffix — a scenario should not have
    to know the host's naming convention.
    """
    return any(name == want or name.endswith("__" + want) for name in called)


def check(scenario, transcript_path):
    called, said = read_transcript(transcript_path)
    problems = []

    for want in scenario.get("must_call", []):
        if not tool_matches(called, want):
            problems.append(
                f"never called {want} — the answer was not drawn from Margince "
                f"(called: {', '.join(sorted(set(called))) or 'nothing'})"
            )

    for pattern in scenario.get("must_mention", []):
        if not re.search(pattern, said, re.IGNORECASE):
            problems.append(f"never said anything matching /{pattern}/")

    for pattern in scenario.get("must_not_mention", []):
        found = re.search(pattern, said, re.IGNORECASE)
        if found:
            problems.append(f"said {found.group(0)!r}, which /{pattern}/ forbids")

    return problems


def main():
    if sys.argv[1] == "--field":
        scenario = parse_scenario(sys.argv[3])
        value = scenario.get(sys.argv[2], "")
        print(value if not isinstance(value, list) else "\n".join(map(str, value)))
        return 0

    if sys.argv[1] == "--record":
        scenario = parse_scenario(sys.argv[2])
        print(json.dumps({
            "scenario": scenario.get("name"),
            "criteria": scenario.get("criteria", []),
            "passed": int(sys.argv[3]),
            "runs": int(sys.argv[4]),
            "pass_at": scenario.get("pass_at"),
        }, indent=2))
        return 0

    if sys.argv[1] == "--check":
        problems = check(parse_scenario(sys.argv[2]), sys.argv[3])
        for problem in problems:
            print(f"  {problem}")
        return 1 if problems else 0

    print(f"unknown mode {sys.argv[1]}", file=sys.stderr)
    return 2


if __name__ == "__main__":
    sys.exit(main())
