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
    r"|the tools?(?:['’]s|s['’])? (?:answer|output|result|reading|data)"
    r"|tool (?:results?|output|answers?)"
    r"|(?:that|which|what) the tools? (?:flag|report|return|say|show)\w*"
    r"|the tools? (?:did not|didn['’]t|does not|doesn['’]t|never) (?:flag|report|return|say|show)\w*"
    r"|(?:reading|read) it made|against (?:the|what the) tools?"
    r"|(?:reported|returned|flagged|shown|listed|given) by the tools?"
    r"|the tools?(?:['’]s|s['’])? (?:figures?|numbers?|values?|totals?|counts?)"
    r"|the tools? (?:flagged|reported|returned|showed|listed|gave)"
    r"|(?:the )?results? of the (?:reads?|lookups?|query|queries|tools?|(?:tool|read|lookup) calls?))\b",
    re.IGNORECASE,
)


# A negation an answer contracts must read the same as the one it spells out:
# a positive pin that misses "isn't" fails a true answer, and an exclusion
# that misses it lets a denial through. So each spelled negation needs, in an
# alternation group around it, a sibling that reads the same with the negation
# contracted: `(?:could not|couldn't) send`, `is(?: not|n't)`.
_APOSTROPHE = r"(?:\[[^\]]*['’.][^\]]*\]\??|['’.]\??)"
_SHORT = re.compile(r"(?i)(?<![A-Za-z\\])((?:\\w[*+]?|\[[^\]]*\][*+?]?|\(\?:[A-Za-z|]*\)|[A-Za-z])*)n"
                    + _APOSTROPHE + "t")
_AUXILIARIES = ("am", "can", "could", "does", "do", "did", "is", "are", "was", "were", "has", "have", "had",
                "will", "would", "shall", "should", "might", "must", "need", "ought")
_SPELLED = re.compile(r"(?i)(?<![A-Za-z'’])(?:(%s)(?:\s[+*?]?)+not|(cannot)|(not))(?![A-Za-z'’])" % "|".join(_AUXILIARIES))
_I_AM = re.compile(r"(?:" + _APOSTROPHE + r")m\s+$")
_STEM = {"cannot": "ca", "can": "ca", "will": "wo", "shall": "sha"}
# Whitespace a pin must match: "was (?:not|n't)" can never read "wasn't".
_SPACE_REQUIRED = re.compile(r"(?:\\s|\s)\+?$")
# What two siblings may differ by and still read the same: spacing, word
# edges and an inline flag such as (?i).
_SPACING = re.compile(r"(?:\\[bBsW]|\s)[?*+]?|\(\?[a-zA-Z]+\)")
_GROUP_PREFIX = re.compile(r"\?(?:[:!=>]|<[=!]|P?<\w+>|[a-zA-Z-]+:)")


def _letters_hidden(source):
    """The pin with every letter escape (\b, \s, \W, \d...) blanked to spaces,
    so an escape beside a `not` reads as the word edge it is."""
    out, index = [], 0
    while index < len(source):
        if source[index] == "\\" and index + 1 < len(source):
            pair = source[index:index + 2]
            out.append("  " if pair[1].isalpha() else pair)
            index += 2
        else:
            out.append(source[index])
            index += 1
    return "".join(out)


def _levels(source):
    """Every alternation level of the pin, innermost first: (start, end) of each
    parenthesized group's content, then the whole pin."""
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
            start = stack.pop() + 1
            prefix = _GROUP_PREFIX.match(source, start)
            spans.append((start - 1, prefix.end() if prefix else start, index))
    return sorted(spans, key=lambda span: span[2] - span[1]) + [(0, 0, len(source))]


def _branches(source, start, end):
    """The (start, end) of each top-level alternative between start and end."""
    depth, escaped, in_class, branches, begin = 0, False, False, [], start
    for index in range(start, end + 1):
        char = source[index] if index < end else "|"
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
            branches.append((begin, index))
            begin = index + 1
    return branches


def _key(text):
    return _SPACING.sub("", text)


def _short_form_fits(stem, prefix):
    """Whether a short form with these leading characters contracts the spelled
    negation: a generic `\w*n't` contracts any, and a spelled word its own."""
    if "\\" in prefix or "[" in prefix:
        return True
    words = _spellings(prefix.lower())
    if stem is None:
        return not words & {"ca", "wo"}
    return stem in words


def _spellings(prefix):
    """The words a prefix such as ha(?:ve|s) spells: {"have", "has"}."""
    group = re.fullmatch(r"([a-z]*)\(\?:([a-z|]*)\)([a-z]*)", prefix)
    if not group:
        return {prefix}
    return {group[1] + middle + group[3] for middle in group[2].split("|")}


def _contracted_beside(source, start, end, stem):
    for opening, low, high in _levels(source):
        if not low <= start < end <= high:
            continue
        branches = _branches(source, low, high)
        if len(branches) < 2:
            continue
        holder = next((b for b in branches if b[0] <= start and end <= b[1]), None)
        if holder is None:
            continue
        spelled = _key(source[holder[0]:start] + "\0" + source[end:holder[1]])
        for low_b, high_b in branches:
            if (low_b, high_b) == holder:
                continue
            sibling = source[low_b:high_b]
            for short in _SHORT.finditer(sibling):
                before = sibling[:short.start()] if short.start() or not opening else source[:opening]
                attached = short.group(1) or not _SPACE_REQUIRED.search(before)
                if attached and _short_form_fits(stem, short.group(1)) and \
                        _key(sibling[:short.start()] + "\0" + sibling[short.end():]) == spelled:
                    return True
    return False


def _negation_problems(data):
    problems = []
    for key in ("must_mention", "must_not_mention"):
        for number, pattern in enumerate(data.get(key, []), 1):
            scanned = _letters_hidden(pattern)
            for match in _SPELLED.finditer(scanned):
                auxiliary = (match.group(1) or match.group(2) or "").lower()
                # English has no short form of "am not".
                if auxiliary == "am" or (not auxiliary and _I_AM.search(scanned[:match.start()])):
                    continue
                stem = _STEM.get(auxiliary, auxiliary) if auxiliary else None
                if not _contracted_beside(pattern, match.start(), match.end(), stem):
                    problems.append(f"{key} entry {number} spells {match.group(0).strip()!r} with no sibling that "
                                    "reads the same contracted; add one in the same group, as (?:was not|wasn't)")
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
