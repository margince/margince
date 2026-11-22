#!/usr/bin/env python3
"""The scenario lint: check.py's own, then the rules on how a pin or a judged
criterion is written.

    scenariolint.py <scenario.yaml>...

The wording rules live here and not in check.py: every byte of check.py is in
the verdicts' harness stamp, and a rule that grades no run must not make every
verdict stale.
"""

import os
import re
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

import check  # noqa: E402

# A judged criterion is read beside the final answer alone, so one that defers
# to what a tool answered asks for a reading the judge cannot make: name the
# expected facts in the criterion instead.
_DEFERS_TO_A_TOOL = re.compile(
    r"\b(?:as the tools? (?:reports?|says?|shows?|returns?|returned|gives?|flagged)"
    r"|the tools?(?:'s)? (?:answer|output|result|reading|data)"
    r"|tool (?:results?|output|answers?)"
    r"|(?:that|which|what) the tools? (?:flag|report|return|say|show)\w*"
    r"|the tools? (?:did not|didn't|does not|doesn't|never) (?:flag|report|return|say|show)\w*"
    r"|(?:reading|read) it made|against (?:the|what the) tools?)\b",
    re.IGNORECASE,
)


# "not" in a pin also reads "isn't", "hasn't" and "couldn't", which no `not`
# matches: a positive pin then fails a true answer, and an exclusion lets a
# negated claim through.
_BARE_NOT = re.compile(r"(?<![\w'’])not(?![\w'’])", re.IGNORECASE)
_CONTRACTED_NOT = re.compile(r"n(?:\[['’]+\]\??|['’]\??)t")


def _negation_problems(data):
    return [f"{key} entry {number} spells a negation as `not` with no n't form; add n['’]t beside it"
            for key in ("must_mention", "must_not_mention")
            for number, pattern in enumerate(data.get(key, []), 1)
            if _BARE_NOT.search(pattern) and not _CONTRACTED_NOT.search(pattern)]


def _judge_problems(data):
    return [f"judge criterion {number} defers to a tool's answer ({match.group(0)!r}), which the judge never "
            "sees: name the expected facts in the criterion"
            for number, criterion in enumerate(data.get("judge", []), 1)
            for match in [_DEFERS_TO_A_TOOL.search(criterion)] if match]


def lint(path):
    """Every problem with one scenario file: the grader's, then the wording rules."""
    problems = check.lint(path)
    if problems:
        return problems
    data = check.parse_scenario(path)
    return _judge_problems(data) + _negation_problems(data)


def main(paths):
    found = 0
    for path in paths:
        for problem in lint(path):
            print(f"{os.path.basename(path)}: {problem}")
            found += 1
    return 1 if found else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
