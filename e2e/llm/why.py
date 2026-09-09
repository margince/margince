#!/usr/bin/env python3
"""Why did a run not call the tool its scenario requires?

A failing `must_call` says which tool was missed and nothing about why. The
answer is almost always one of three things, and they need different fixes:

  the tool was never served        -> a scope, a tier floor, or a registration
  it was served and not chosen     -> the tool's own copy, read beside its rivals
  it was chosen and refused        -> the refusal, which the transcript carries

This prints the evidence for all three, per run, so the question is answered by
reading rather than reconstructed by hand each time. Reads only what the lane
already records; it drives no model and costs nothing.

    e2e/llm/why.py case20_put_it_in_the_board_pack
"""

import json
import pathlib
import sys

ROOT = pathlib.Path(__file__).resolve().parents[2]
RECORDS = ROOT / "e2e" / "llm" / "records"
SCENARIOS = ROOT / "e2e" / "llm" / "scenarios"
SERVED = ROOT / "docs" / "reference" / "mcp-info.json"


def must_call(scenario):
    """The tools the scenario requires, read off its own YAML.

    Parsed by hand rather than with a YAML library: the patterns in these files
    carry regex escapes a strict loader rejects, and this only needs one list.
    """
    for path in SCENARIOS.glob("*.yaml"):
        if f"name: {scenario}" not in path.read_text():
            continue
        wanted, inside = [], False
        for line in path.read_text().splitlines():
            if line.startswith("must_call:"):
                inside = True
                continue
            if inside:
                if line.startswith("  - "):
                    wanted.append(line[4:].strip())
                elif line and not line.startswith((" ", "#")):
                    break
        return wanted
    return []


def served_descriptions():
    """Every tool the surface publishes, by bare name."""
    if not SERVED.exists():
        return {}
    out = {}

    def walk(node):
        if isinstance(node, dict):
            name, text = node.get("name"), node.get("description")
            if isinstance(name, str) and isinstance(text, str):
                out[name] = text
            for value in node.values():
                walk(value)
        elif isinstance(node, list):
            for value in node:
                walk(value)

    walk(json.load(SERVED.open()))
    return out


def read_run(path):
    """One run's served tool list, its calls, and its final answer."""
    offered, called, answer = [], [], ""
    for line in path.read_text().splitlines():
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            continue
        if event.get("type") == "system" and event.get("subtype") == "init":
            offered = [t.split("__")[-1] for t in event.get("tools") or []]
        if event.get("type") == "assistant":
            for block in event.get("message", {}).get("content", []):
                if block.get("type") == "tool_use":
                    called.append(block["name"].split("__")[-1])
                elif block.get("type") == "text" and block["text"].strip():
                    answer = block["text"]
    return offered, called, answer


def main():
    if len(sys.argv) != 2:
        sys.exit(__doc__)
    scenario = sys.argv[1]
    wanted = must_call(scenario)
    descriptions = served_descriptions()
    runs = sorted(RECORDS.glob(f"{scenario}.run*.jsonl"))
    if not runs:
        sys.exit(f"no transcripts for {scenario} under {RECORDS}")

    print(f"{scenario}: must_call {wanted or '(none)'}\n")
    missed_anywhere = set()
    for path in runs:
        offered, called, answer = read_run(path)
        missed = [t for t in wanted if t not in called]
        missed_anywhere.update(missed)
        print(f"--- {path.name}")
        print(f"    served : {len(offered)} tools")
        print(f"    called : {', '.join(called) or '(nothing)'}")
        for tool in missed:
            where = "SERVED and not chosen" if tool in offered else "NEVER SERVED"
            print(f"    missed : {tool} — {where}")
        print(f"    answer : {answer[:160].replace(chr(10), ' ')}")
        print()

    for tool in sorted(missed_anywhere):
        text = descriptions.get(tool)
        if not text:
            continue
        # The copy is what a model chose against, so it is printed whole rather
        # than summarised: the first sentence is what a scan reads.
        print(f"=== {tool} reads, to a caller scanning for a job:")
        print("    " + text.replace("\n", " ")[:600])
        print()


if __name__ == "__main__":
    main()
