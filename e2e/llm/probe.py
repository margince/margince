#!/usr/bin/env python3
"""Judge candidate answers against ONE scenario's guards, and say which guard
decided.

WHY THIS EXISTS. A scenario's must_mention/must_not_mention entries are
hand-written regexes over free prose, and prose is not a thing a human can hold
in their head. Four review rounds of case 6 each found the same two defects by
constructing sentences by hand: a pattern that RED a correct answer, and a
pattern that MISSED the defect it exists for — including regressions introduced
by the previous round's own fix. Constructing sentences by hand does not scale
and it leaked every time. This makes the same question cheap to ask: hand it a
sentence and it says exactly which pattern fired or failed to.

THE JUDGE IS check.check ITSELF. This builds a real stream-json transcript,
satisfies the scenario's must_call/must_call_with so the tool half is out of the
way, and runs it through the same function the paid lane runs. A probe that
re-implemented the matching would be a second answer to the question the lane
already answers, and the two would drift — at which point a green probe would
say nothing about a green lane. Attribution back to individual patterns is done
by reconstructing check.check's own message for each pattern, not by matching
the pattern again here.

WHAT IT DOES NOT DO is decide whether an answer is correct. That is the caller's
claim, made with --expect, and the probe only reports whether the guards agree
with it:

    --expect correct    green is expected; a red one is a FALSE RED
    --expect defective  red is expected; a green one is a FALSE GREEN

Exit 1 on any such finding, so a script or a CI step can use it; exit 2 when the
probe itself could not run.

Usage:

    probe.py <scenario.yaml> --expect correct "answer one" "answer two"
    probe.py <scenario.yaml> --jsonl < candidates.jsonl
    probe.py <scenario.yaml> --brief --count 6      # the generation brief

A SCENARIO'S JUDGED CRITERIA ARE JUDGED HERE TOO, because this judges with
check.check and that is where they live. So auditing a scenario that carries any
costs tokens per candidate and needs E2E_LLM_JUDGE set — which is the honest
price of asking whether the whole verdict is sound rather than half of it. A
judge that cannot be reached raises rather than scoring, so a candidate is never
reported green on a criterion nobody decided.

Stdlib only, for check.py's own reason: the lane must run on a fresh checkout,
and a PyYAML dependency in front of a scenario file would make it refuse to.
"""

import argparse
import json
import os
import re
import sys
import tempfile

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

import check  # the judge itself, imported after its directory is on the path


def _tool_calls(scenario):
    """The tool_use blocks that satisfy this scenario's tool half.

    The probe asks about PROSE. A transcript that failed must_call would report
    a problem for every candidate answer alike and drown the halves being
    measured, so the tool calls the scenario demands are all present.

    must_call_with values are written back verbatim, which is exactly what
    check.check compares against: it renders a string argument as itself and
    anything else as compact JSON, so the declared value re-read as a string is
    equal to itself either way. Nothing here is testing must_call_with — it is
    being satisfied, and the simplest thing that satisfies it is the least to go
    wrong.
    """
    calls = [
        {"type": "tool_use", "name": "mcp__margince__" + tool, "input": {}}
        for tool in scenario.get("must_call", [])
    ]
    for spec in scenario.get("must_call_with", []):
        target, _, value = spec.partition("=")
        tool, _, argument = target.partition(".")
        if not tool or not argument:
            continue
        calls.append(
            {"type": "tool_use", "name": "mcp__margince__" + tool, "input": {argument: value}}
        )
    return calls


def _transcript(scenario, answer):
    """Write one candidate answer as a stream-json transcript and return its path.

    The answer is carried in BOTH an assistant text block and the terminal
    result, because that is the shape a real run has and check.read_transcript
    reads both — a probe that used only one would judge a different string than
    the lane does.
    """
    calls = _tool_calls(scenario)
    handle = tempfile.NamedTemporaryFile(
        "w", suffix=".jsonl", delete=False, dir=tempfile.gettempdir(), encoding="utf-8"
    )
    with handle as out:
        if calls:
            out.write(json.dumps({"type": "assistant", "message": {"content": calls}}) + "\n")
        out.write(
            json.dumps(
                {"type": "assistant", "message": {"content": [{"type": "text", "text": answer}]}}
            )
            + "\n"
        )
        out.write(
            json.dumps({"type": "result", "subtype": "success", "is_error": False, "result": answer})
            + "\n"
        )
    return handle.name


def judge(scenario, answer):
    """Run one answer through check.check and attribute each problem to a pattern.

    Returns (missed, fired, refused, unattributed): the must_mention patterns
    that found nothing, the must_not_mention patterns that fired paired with the
    text they matched, the judged criteria the judge answered no to, and
    anything else check.check said — which should be empty, and is reported
    rather than swallowed when it is not.
    """
    path = _transcript(scenario, answer)
    try:
        problems = list(check.check(scenario, path))
    finally:
        os.unlink(path)

    missed, fired, refused = [], [], []
    for pattern in scenario.get("must_mention", []):
        message = f"never said anything matching /{pattern}/"
        if message in problems:
            problems.remove(message)
            missed.append(pattern)
    for pattern in scenario.get("must_not_mention", []):
        suffix = f", which /{pattern}/ forbids"
        hit = next((p for p in problems if p.startswith("said ") and p.endswith(suffix)), None)
        if hit is not None:
            problems.remove(hit)
            fired.append((pattern, hit[len("said ") : -len(suffix)]))
    # The semantic half. A judged criterion carries its own words, so the
    # attribution is the criterion itself rather than a pattern head.
    for problem in list(problems):
        if problem.startswith(check.JUDGED_NO):
            problems.remove(problem)
            refused.append(problem[len(check.JUDGED_NO) :].strip())
    return missed, fired, refused, problems


def _abbreviate(pattern, width=110):
    """A pattern short enough to read in a report line.

    The full regex is often several hundred characters, and a report that prints
    all of it buries the finding. The head is what identifies which entry it is;
    `--full` prints the whole thing when the reader wants to edit it.
    """
    single = pattern.replace("\n", " ")
    return single if len(single) <= width else single[: width - 1] + "…"


def _excerpt(answer, width=100):
    single = " ".join(answer.split())
    return single if len(single) <= width else single[: width - 1] + "…"


def report(scenario, answer, expect, full, out=sys.stdout):
    """Print one candidate's verdict and return True when it is a finding."""
    missed, fired, refused, unattributed = judge(scenario, answer)
    green = not missed and not fired and not refused and not unattributed
    show = (lambda p: p) if full else _abbreviate

    if expect == "correct":
        finding = not green
        headline = "FALSE RED " if finding else "ok, green"
    elif expect == "defective":
        finding = green
        headline = "FALSE GREEN" if finding else "ok, caught"
    else:
        finding = False
        headline = "green     " if green else "red       "

    print(f"{headline}  {_excerpt(answer)}", file=out)
    for pattern in missed:
        print(f"    must_mention MISSED     /{show(pattern)}/", file=out)
    for pattern, matched in fired:
        print(f"    must_not_mention FIRED  /{show(pattern)}/", file=out)
        print(f"      on: {matched}", file=out)
    for criterion in refused:
        print(f"    judge SAID NO           {criterion}", file=out)
    for problem in unattributed:
        # Not a prose finding: the transcript this probe built failed a check it
        # was supposed to satisfy. Said out loud, because silently counting it
        # as a red answer would make every candidate for that scenario a FALSE
        # RED and hide the real cause.
        print(f"    PROBE FAULT             {problem}", file=out)
    return finding


def _candidates(args):
    """The answers to judge, each paired with what the caller claims about it.

    Three sources, because the two callers want different ones: a human types
    sentences as arguments, and the generation round pipes JSONL whose objects
    carry their own expectation. Lines that are not a candidate object are
    skipped rather than fatal — a model asked for JSONL wraps it in a code fence
    often enough that refusing would throw away the whole round.
    """
    if args.jsonl:
        found = []
        for line in sys.stdin:
            try:
                item = json.loads(line)
            except json.JSONDecodeError:
                continue
            if isinstance(item, dict) and isinstance(item.get("text"), str):
                found.append((item["text"], item.get("expect")))
        return found
    if args.answer:
        return [(answer, args.expect) for answer in args.answer]
    text = sys.stdin.read()
    return [(block.strip(), args.expect) for block in text.split("\n---\n") if block.strip()]


# The keys whose entries are the regexes themselves. The brief keeps the
# scenario's COMMENTS — the author's plain-English statement of what the answer
# must find and what it must not claim — and drops the patterns, so the model
# writes an answer to the case rather than an answer to the regex. A model shown
# the pattern optimises against it, and candidates written that way would prove
# only that the pattern matches itself.
#
# `judge:` is NOT dropped, and the difference is the point. A judged criterion is
# the case stated in words — the same thing the comments are — so a model shown
# it writes an answer to the case. There is no pattern behind it to optimise
# against.
_PATTERN_KEYS = ("must_mention", "must_not_mention")


def _brief_scenario(path):
    """The scenario file with its guard patterns removed."""
    kept, key = [], None
    # check's own reader, so the probe refuses the same paths the lane refuses
    # rather than growing a second answer to where a scenario may live.
    for line in check._open_checked(path).read().splitlines():
        match = re.match(r"^([A-Za-z_][A-Za-z0-9_]*):", line)
        if match:
            key = match.group(1)
        if key in _PATTERN_KEYS and line.startswith("  - "):
            continue
        kept.append(line)
    return "\n".join(kept)


def brief(path, count):
    """The prompt that asks a model for candidate answers to this scenario.

    DERIVED FROM THE SCENARIO THE PROBE JUDGES AGAINST, rather than written out
    beside it, so the round cannot ask about a case it does not measure and
    cannot go stale when the case is rewritten.
    """
    scenario = check.parse_scenario(path)
    return f"""You are helping audit the REGEX GUARDS of one end-to-end test scenario.

A test harness drives a CRM assistant with the prompt below, then judges the
assistant's prose with hand-written regexes. Those regexes are what is on trial
here — not the assistant. To test them we need realistic answers of two kinds.

Here is the scenario file, with the regexes themselves removed. Its comments
state what a correct answer must find and what a wrong answer claims:

--- scenario ---
{_brief_scenario(path)}
--- end scenario ---

Write {count} answers a GOOD assistant would give to that prompt — correct on
every point the file says matters — and {count} answers that are fluent and
plausible but contain the SPECIFIC defect the file forbids, and only that
defect.

Rules:
- Write whole answers, the length and shape a real assistant writes: several
  sentences, sometimes bullets, sometimes a table, sometimes a heading.
- Vary the register hard. Split a finding over two sentences. Quote a source.
  Use the German wording where the fixture is German. Hedge in one, be blunt in
  another. The point is to find the phrasings the guards have not met.
- Do NOT reuse the example phrasings quoted in the file's comments — those are
  already known. Invent fresh ones.
- A "defective" answer must otherwise be a good answer. One that also forgets
  everything else proves nothing about the guard for the defect.

Output JSONL and nothing else — one object per line, no code fence, no prose:
{{"expect": "correct", "text": "..."}}
{{"expect": "defective", "text": "..."}}

The scenario is {scenario.get("name")}."""


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("scenario")
    parser.add_argument("answer", nargs="*")
    parser.add_argument("--expect", choices=("correct", "defective"))
    parser.add_argument("--jsonl", action="store_true", help="read candidate objects from stdin")
    parser.add_argument("--full", action="store_true", help="print whole patterns, not their heads")
    parser.add_argument("--brief", action="store_true", help="print the generation brief and exit")
    parser.add_argument("--count", type=int, default=6, help="answers per kind, for --brief")
    # INTERMIXED, because `answer` is nargs="*" and the callers put options
    # between the scenario and the answers — `probe.py case.yaml --expect correct
    # "..."`. Plain parse_args assigns trailing positionals only on some Python
    # versions and answers "unrecognized arguments" on others, so the same
    # command worked locally and failed on CI. parse_intermixed_args is the
    # documented way to accept that shape on every version.
    args = parser.parse_intermixed_args()

    if args.brief:
        print(brief(args.scenario, args.count))
        return 0

    scenario = check.parse_scenario(args.scenario)
    candidates = _candidates(args)
    if not candidates:
        print("no candidate answers were given", file=sys.stderr)
        return 2

    findings = 0
    for answer, expect in candidates:
        if report(scenario, answer, expect, args.full):
            findings += 1
    print(f"\n{len(candidates)} candidates, {findings} findings")
    return 1 if findings else 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except check.judge.JudgeUnavailable as unavailable:
        # Exit 2, the probe's own "could not run": a scenario carrying judged
        # criteria cannot be audited without a judge, and reporting the
        # candidates green on the half nobody decided would be the finding this
        # tool exists to prevent, made by the tool itself.
        print(f"the judge could not be reached: {unavailable}", file=sys.stderr)
        sys.exit(2)
