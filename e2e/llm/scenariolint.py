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
    r"|(?:reading|read) it made|against (?:the|what the) tools?"
    r"|(?:reported|returned|flagged|shown|listed|given) by the tools?"
    r"|the tools?(?:'s|s'|’s)? (?:figures?|numbers?|values?|totals?|counts?)"
    r"|the tools? (?:flagged|reported|returned|showed|listed|gave)"
    r"|(?:the )?results? of the (?:read|reads|call|calls|lookup|query|tool))\b",
    re.IGNORECASE,
)


# A negation an answer contracts must read the same as the one it spells out:
# a positive pin that misses "isn't" fails a true answer, and an exclusion
# that misses it lets a denial through. Each spelled negation is held on its
# own: an auxiliary one ("could not", "cannot") needs its own short form
# ("couldn't", "can't"), and a bare `not` needs a generic n't in a group
# around it, not merely somewhere in the pin.
_APOSTROPHE = r"(?:\[['’]+\]\??|['’]\??)"
_GENERIC_NT = re.compile(r"(?<![A-Za-z])n" + _APOSTROPHE + "t")
# Beside a bare `not`, "wasn't" or "isn't" is its short form too; "can't" and
# "won't" stand only for their own verbs.
_ANY_NT = re.compile(r"(?<!ca)(?<!wo)n" + _APOSTROPHE + "t", re.IGNORECASE)
_AUXILIARIES = ("can", "could", "does", "do", "did", "is", "are", "was", "were", "has", "have", "had",
                "will", "would", "should", "must", "need")
_SPELLED = re.compile(r"(?i)(?<![A-Za-z'’])(?:(%s) ?not|(cannot)|(not))(?![A-Za-z'’])" % "|".join(_AUXILIARIES))
_SHORT_FORM = {"can": "can", "will": "won"}


def _groups_around(source, at):
    """The text of every parenthesized group that encloses index at, innermost first."""
    stack, spans, escaped, in_class = [], [], False, False
    for index, char in enumerate(source):
        if escaped:
            escaped = False
        elif char == "\\":
            escaped = True
        elif in_class:
            in_class = char != "]"
        elif char == "[":
            in_class = True
        elif char == "(":
            stack.append(index)
        elif char == ")" and stack:
            start = stack.pop()
            if start < at < index:
                spans.append((start, index))
    return [source[start:end + 1] for start, end in sorted(spans, key=lambda span: span[1] - span[0])]


def _branches_beside(source, at):
    """The top-level alternative of the pin that holds index at, and the ones
    on either side of it: a short form listed next to its spelled form."""
    depth, start, escaped, in_class, branches, holder = 0, 0, False, False, [], 0
    for index, char in enumerate(source + "|"):
        if escaped:
            escaped = False
        elif char == "\\":
            escaped = True
        elif in_class:
            in_class = char != "]"
        elif char == "[":
            in_class = True
        elif char == "(":
            depth += 1
        elif char == ")":
            depth -= 1
        elif char == "|" and depth == 0:
            if start <= at < index:
                holder = len(branches)
            branches.append(source[start:index])
            start = index + 1
    return branches[max(0, holder - 1):holder + 2]


def _short_form_near(source, at, auxiliary):
    if auxiliary:
        word = "can" if auxiliary.lower() == "cannot" else auxiliary.lower()
        short = _SHORT_FORM.get(word, re.escape(word) + "n")
        attached = re.compile("(?i)" + short + _APOSTROPHE + "t")
        grouped = re.compile("(?i)" + re.escape(word) + r"\(\?:[^()]*?" + _GENERIC_NT.pattern + r"[^()]*\)")
        return bool(attached.search(source) or grouped.search(source))
    nearby = _groups_around(source, at) or _branches_beside(source, at)
    return any(_ANY_NT.search(text) for text in nearby)


def _negation_problems(data):
    problems = []
    for key in ("must_mention", "must_not_mention"):
        for number, pattern in enumerate(data.get(key, []), 1):
            source = pattern.replace("\\b", "  ").replace("\\B", "  ")
            for match in _SPELLED.finditer(source):
                auxiliary = match.group(1) or match.group(2)
                if not _short_form_near(source, match.start(), auxiliary):
                    problems.append(f"{key} entry {number} spells {match.group(0)!r} with no short form of it; "
                                    "add the n't form beside it")
    return problems


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
