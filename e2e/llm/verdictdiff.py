#!/usr/bin/env python3
"""The table a reviewer of the weekly verdicts decides with.

    verdictdiff.py BEFORE_DIR AFTER_DIR

How many scenarios have a verdict, and one row per scenario whose result moved:
passed of runs before and after, and whether it held its bar. Printed as
Markdown for the pull request body, so nobody has to read 73 JSON diffs.
"""

import json
import os
import sys


def _read(directory):
    out = {}
    if not os.path.isdir(directory):
        return out
    for file in sorted(os.listdir(directory)):
        if file.endswith(".json"):
            with open(os.path.join(directory, file), encoding="utf-8") as handle:
                verdict = json.load(handle)
            out[verdict["scenario"]] = verdict
    return out


def _result(verdict):
    if verdict is None:
        return "none"
    held = "held" if verdict.get("pass_at") and verdict["passed"] >= verdict["pass_at"] else "below its bar"
    return f"{verdict['passed']} of {verdict['runs']}, {held}"


def table(before, after):
    old, new = _read(before), _read(after)
    held = sum(1 for v in new.values() if v.get("pass_at") and v["passed"] >= v["pass_at"])
    lines = [f"{len(new)} scenarios have a verdict; {held} held their bar. Search: "
             f"{', '.join(sorted({str(v.get('search', 'unknown')) for v in new.values()})) or 'none'}.", ""]
    moved = [name for name in sorted(new) if _result(old.get(name)) != _result(new[name])]
    if not moved:
        return "\n".join(lines + ["No scenario changed its result."]) + "\n"
    lines += ["| Scenario | Before | After |", "|---|---|---|"]
    lines += [f"| `{name}` | {_result(old.get(name))} | {_result(new[name])} |" for name in moved]
    return "\n".join(lines) + "\n"


if __name__ == "__main__":
    if len(sys.argv) != 3:
        print(__doc__, file=sys.stderr)
        sys.exit(2)
    print(table(sys.argv[1], sys.argv[2]), end="")
