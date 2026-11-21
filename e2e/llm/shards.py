#!/usr/bin/env python3
"""Split the scenario folder into shards for the weekly run, and check what came back.

    shards.py plan N        a GitHub matrix: N shards of case numbers, from the folder
    shards.py collect DIR   exit 0 when DIR holds a fresh verdict for every scenario

The shares come from the folder listing, never from a typed list, so a new case
is never left out of a shard. Cases are dealt round-robin in case-number order,
so the long and the short spread across the shards.
"""

import json
import os
import re
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

import check  # noqa: E402

SCENARIOS = os.path.join(os.path.dirname(os.path.abspath(__file__)), "scenarios")


def scenarios(directory=SCENARIOS):
    """(case number, scenario name) for every scenario file, in case-number order."""
    found = []
    for file in os.listdir(directory):
        number = re.match(r"case(\d+)-.*\.yaml$", file)
        if not number:
            if file.endswith(".yaml"):
                raise ValueError(f"{file} has no case number, so no shard can name it")
            continue
        found.append((int(number.group(1)), check.parse_scenario(os.path.join(directory, file))["name"]))
    return sorted(found)


def plan(count, directory=SCENARIOS):
    cases = scenarios(directory)
    shards = [[] for _ in range(min(count, len(cases)))]
    for index, (number, _) in enumerate(cases):
        shards[index % len(shards)].append(str(number))
    return {"include": [{"shard": i + 1, "cases": " ".join(numbers)} for i, numbers in enumerate(shards)]}


def missing(verdicts, directory=SCENARIOS):
    """The scenarios with no verdict file in `verdicts`."""
    have = {file[: -len(".json")] for file in os.listdir(verdicts) if file.endswith(".json")}
    return [name for _, name in scenarios(directory) if name not in have]


def main(argv):
    if len(argv) == 3 and argv[1] == "plan":
        print(json.dumps(plan(int(argv[2]))))
        return 0
    if len(argv) == 3 and argv[1] == "collect":
        absent = missing(argv[2])
        if absent:
            print(f"{len(absent)} of {len(scenarios())} scenarios have no verdict from this run: {' '.join(absent)}")
            return 1
        print(f"{len(scenarios())} of {len(scenarios())} scenarios have a verdict from this run")
        return 0
    print(__doc__, file=sys.stderr)
    return 2


if __name__ == "__main__":
    sys.exit(main(sys.argv))
