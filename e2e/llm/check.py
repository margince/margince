#!/usr/bin/env python3
"""Read a scenario, and judge one run of it against what the assistant said.

Four things are checked per run, and all four must hold:

  must_call        Did it reach the tools it needed? An answer written from the
                   model's own memory, with Margince never asked, is a failure
                   however good the prose reads.
  must_mention     Does the answer contain what it has to contain? Each entry is
                   a regex, and alternatives inside one entry are an any-of
                   group.
  must_not_mention Does it avoid what it must avoid? This half matters more than
                   the first: a confident wrong answer is worse than no answer.
  judge            Does the answer do what this sentence says? Each entry is a
                   criterion in plain words, decided by a model (judge.py).

WHICH HALF AN ASSERTION BELONGS IN is the whole design, and it was learned by
measurement. A regex is reliable for a MECHANICAL fact — the answer carries this
name, this date, this count, this identifier — and every leak five rounds of
human review and two paid guard sweeps found was in the other kind: did the
answer notice that the note and the record disagree, did it state the limit
rather than claim the customer is free, did it report the row it could not
import. Those are SEMANTIC and belong in `judge`, where the scenario states the
question in a sentence that is both the prompt and the documentation. Adding one
more alternative to a regex that has already leaked five times is the move this
lane has measured not working.

A JUDGED CRITERION NOBODY JUDGED IS NOT A PASS. judge.py has no default backend
and no fallback: every path that cannot produce a real verdict raises
JudgeUnavailable, which arrives here as exit 2 and stops the lane. A silent pass
would be this lane reporting green having checked nothing.

Deliberately NOT judged: wording, tone, length, formatting, the order it did
things in, or extra correct information. Only whether the facts are right and
the required things were said.

The patterns themselves are the part of this that nothing else measures — a
must_mention that misses the finding reports PASS with no failing assertion to
notice. probe.py beside this file puts one on trial: it judges candidate answers
through check() itself and names the pattern that decided.

Stdlib only — no PyYAML. The scenario files are a small fixed subset of YAML
(scalars, block strings, flat lists) and a dependency for that would make the
lane refuse to run on a fresh checkout. That holds with the judge too: judge.py
reaches its model through the `claude` CLI, so nothing here imports an SDK, and
the offline self-test replays recorded verdicts with neither a credential nor a
network.
"""

import json
import os
import re
import sys
import tempfile

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

import judge  # noqa: E402  — the semantic half, imported after its directory is on the path

# The head of the problem a judged criterion reports. Named rather than spelled
# twice: probe.py attributes a problem back to the criterion that produced it by
# this prefix, and a probe that could not recognise one would file it under
# "PROBE FAULT" and read as a fault in the probe rather than a verdict.
JUDGED_NO = "the judge says NO to:"

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

        # No \s* after the colon: the value is stripped two lines down, so it
        # only made the pattern ambiguous about which of the two ate the spaces.
        m = re.match(r"^([A-Za-z_][A-Za-z0-9_]*):(.*)$", line)
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
    called, said, calls = [], [], []
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
                # The ARGUMENTS too, for must_call_with. A scenario that checks
                # only the tool name passes on a call to the right tool with the
                # wrong plan, and must_mention only sees the closing prose — so
                # an unrelated call plus an invented number would score green.
                calls.append((block.get("name", ""), block.get("input") or {}))
            elif block.get("type") == "text":
                said.append(block.get("text", ""))
    return called, "\n".join(said), calls


# The usage fields the terminal `result` event carries, and the ONLY ones read.
# Named rather than summed by iterating whatever the event happens to hold: the
# CLI's usage object grows fields between versions, and a total over "every
# integer under usage" would silently start counting a new one — a number whose
# meaning changed without its name changing, which is the defect this lane was
# rebuilt around.
_USAGE_FIELDS = (
    "input_tokens",
    "output_tokens",
    "cache_creation_input_tokens",
    "cache_read_input_tokens",
)


def read_usage(path):
    """What one run cost: (usage dict, cost in USD), or None when unmeasurable.

    NONE IS NOT ZERO, and the caller may not treat it as zero. A transcript with
    no terminal `result` event is a run whose cost this cannot see — a crash, a
    truncated file, a CLI that changed the event's name — and folding that into
    a total as 0 would report a cheaper sweep than the one that was paid for.
    The lane counts how many runs it actually measured and prints both numbers,
    so an under-read announces itself instead of passing as a low bill.
    """
    for line in _open_checked(path):
        line = line.strip()
        if not line:
            continue
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            continue
        if event.get("type") != "result":
            continue
        usage = event.get("usage")
        if not isinstance(usage, dict):
            return None
        counted = {field: usage.get(field, 0) for field in _USAGE_FIELDS}
        if not all(isinstance(value, int) for value in counted.values()):
            return None
        cost = event.get("total_cost_usd")
        return counted, (cost if isinstance(cost, (int, float)) else None)
    return None


def total_usage(paths):
    """Sum read_usage over several runs, carrying how many were measurable.

    `runs_measured` sits beside the totals rather than being checked here,
    because this cannot know how many runs the caller expected. The caller
    knows, and reporting the pair is what makes a short count visible: a total
    without a denominator is exactly the shape the coverage page failed in.
    """
    totals = dict.fromkeys(_USAGE_FIELDS, 0)
    cost, measured, costed = 0.0, 0, 0
    for path in paths:
        read = read_usage(path)
        if read is None:
            continue
        usage, run_cost = read
        for field in _USAGE_FIELDS:
            totals[field] += usage[field]
        measured += 1
        if run_cost is not None:
            cost += run_cost
            costed += 1
    # The cost is omitted rather than reported as 0.0 when no run carried one:
    # a lane that reads "cost_usd: 0" would say the sweep was free.
    return {
        **totals,
        "runs_measured": measured,
        "cost_usd": round(cost, 4) if costed else None,
    }


def usage_line(expected, usage):
    """The report's one line for a scenario's cost.

    MEASURED OUT OF EXPECTED, always both. A sweep whose transcripts went
    missing would otherwise print a small total and read as a cheap run rather
    than an unread one — the same misreading as a coverage page whose count
    agreed while its sets did not.
    """
    measured = usage["runs_measured"]
    cost = usage["cost_usd"]
    short = "" if measured == expected else f"  <-- {expected - measured} run(s) unmeasured"
    return (
        f"  usage: {measured}/{expected} runs measured, "
        f"{usage['input_tokens']:,} in, {usage['output_tokens']:,} out, "
        f"{usage['cache_read_input_tokens']:,} cache-read, "
        f"cost {'unreported' if cost is None else f'${cost:.2f}'}{short}"
    )


# The failures that mean the model was never reached, rather than that it
# answered badly. Matched on the message because the transport reports both the
# same way — `is_error` on the terminal result — and only the message says which
# happened.
#
# WORD BOUNDARIES on the numeric codes, because a bare substring finds `401`
# inside `4011` and `1401`. A run that reached the model and then failed with a
# number that happens to contain one would be reported as never having happened,
# which is this defect inverted: a real finding discarded and the rest of the
# lane abandoned with it.
#
# The family is deliberately NARROW, and the cost is stated rather than fixed: a
# refusal whose message names no marker — a bare "Permission denied" — is scored
# as a scenario failure. That is the safe direction. Widening the family to
# catch it would also catch a mid-run tool permission error, which IS a finding,
# and excusing one of those is worse than reading one credential problem as a
# bad answer.
_REFUSALS = re.compile(
    r"\b(?:401|403)\b|api key|authenticate|authentication|unauthorized", re.IGNORECASE
)


def unrun(path):
    """Why this run never happened, or "" when it did.

    A scenario failing every criterion and a run that never reached the model
    produce the same score, and only one of them is a finding about the product.
    That is not hypothetical: an expired key answered `401 API key is invalid`
    on all eighteen runs of a lane, every scenario was recorded as failing its
    criteria, and the verdict said six use cases were broken.

    ONLY those two shapes. A transcript with no assistant turn never got as far
    as the model, and a terminal error naming a credential refusal never got
    past the door. Every OTHER `is_error` is a run that happened — the lane sets
    `--max-turns 20`, and exhausting it is a finding about the scenario, not
    about the harness. Excusing one of those would be this same defect inverted:
    a real answer thrown away as a harness fault, and the rest of the lane
    abandoned with it.
    """
    saw_assistant, called_tool = False, False
    failure = ""
    for line in _open_checked(path):
        line = line.strip()
        if not line:
            continue
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            continue
        if event.get("type") == "assistant":
            saw_assistant = True
            for block in (event.get("message") or {}).get("content") or []:
                if isinstance(block, dict) and block.get("type") == "tool_use":
                    called_tool = True
        if event.get("type") == "result" and event.get("is_error"):
            failure = str(event.get("result") or "")
    if not saw_assistant:
        return "the transcript carries no assistant turn: the model was never reached"
    # A REFUSAL AFTER A TOOL CALL IS NOT A REFUSAL AT THE DOOR. The credential
    # that shipped this defect produced one assistant turn carrying the error
    # text and called nothing — the model was never reached. A tool answering
    # `401 Unauthorized` mid-run is the opposite: the model was reached, chose
    # a tool, and the product refused it, which is a finding about the product.
    #
    # The tool call is what separates them, and it is structural rather than a
    # guess at wording.
    if failure and not called_tool and _REFUSALS.search(failure):
        return failure
    return ""


def tool_matches(called, want):
    """A tool is reached when any call names it.

    MCP tools arrive prefixed (mcp__margince__query_workspace), so the scenario
    names the bare tool and this matches the suffix — a scenario should not have
    to know the host's naming convention.
    """
    return any(name == want or name.endswith("__" + want) for name in called)


def check(scenario, transcript_path):
    called, said, calls = read_transcript(transcript_path)
    problems = []

    for want in scenario.get("must_call", []):
        if not tool_matches(called, want):
            problems.append(
                f"never called {want} — the answer was not drawn from Margince "
                f"(called: {', '.join(sorted(set(called))) or 'nothing'})"
            )

    # must_call_with entries are `tool.argument=value`, flat rather than nested
    # because parse_scenario reads a deliberately small YAML subset. The value is
    # compared against the argument rendered as JSON without quotes for a plain
    # string, so both `report=activities-by-kind` and `group_by=["direction"]`
    # are expressible.
    for spec in scenario.get("must_call_with", []):
        target, _, expected = spec.partition("=")
        tool, _, argument = target.partition(".")
        if not tool or not argument:
            problems.append(f"must_call_with entry {spec!r} is not tool.argument=value")
            continue
        seen = []
        for name, arguments in calls:
            if not tool_matches([name], tool):
                continue
            actual = arguments.get(argument)
            rendered = actual if isinstance(actual, str) else json.dumps(actual, separators=(",", ":"))
            seen.append(rendered)
            if rendered == expected:
                break
        else:
            problems.append(
                f"never called {tool} with {argument}={expected} — "
                f"saw {', '.join(repr(s) for s in seen) or 'no such call'}"
            )

    for pattern in scenario.get("must_mention", []):
        if not re.search(pattern, said, re.IGNORECASE):
            problems.append(f"never said anything matching /{pattern}/")

    for pattern in scenario.get("must_not_mention", []):
        found = re.search(pattern, said, re.IGNORECASE)
        if found:
            problems.append(f"said {found.group(0)!r}, which /{pattern}/ forbids")

    # THE SEMANTIC HALF, last because it is the only one that can cost money and
    # the only one that can raise. judge.verdict never answers "no" for a judge
    # it could not reach — that is JudgeUnavailable, and it propagates out of
    # here rather than being caught and scored, because a criterion nobody
    # decided is not a criterion the answer failed.
    for criterion in scenario.get("judge", []):
        held, reason = judge.verdict(criterion, said)
        if not held:
            problems.append(f"{JUDGED_NO} {criterion}\n      the judge's reason: {reason}")

    return problems


def main():
    if sys.argv[1] == "--field":
        scenario = parse_scenario(sys.argv[3])
        value = scenario.get(sys.argv[2], "")
        print(value if not isinstance(value, list) else "\n".join(map(str, value)))
        return 0

    if sys.argv[1] == "--record":
        scenario = parse_scenario(sys.argv[2])
        runs = int(sys.argv[4])
        # The transcripts of THIS scenario's runs, so the committed verdict
        # carries what the answer cost as well as whether it held. They are
        # optional: the offline harness records verdicts with no transcripts at
        # all, and a lane that refused to record without them would make the
        # cheap path depend on the paid one.
        usage = total_usage(sys.argv[5:]) if len(sys.argv) > 5 else None
        record = {
            "scenario": scenario.get("name"),
            "criteria": scenario.get("criteria", []),
            "passed": int(sys.argv[3]),
            "runs": runs,
            "pass_at": scenario.get("pass_at"),
        }
        if usage is not None:
            # runs_measured rides WITH the totals into the committed record. A
            # later reader summing cost across scenarios can then tell a cheap
            # sweep from one whose transcripts it could not read, which the
            # totals alone cannot say.
            record["usage"] = usage
        print(json.dumps(record, indent=2))
        return 0

    if sys.argv[1] == "--usage":
        # One line of prose for the lane's report, formatted here because this
        # file owns what a usage total means — the shell would otherwise parse
        # the JSON and spell the same shape a second time.
        print(usage_line(int(sys.argv[2]), total_usage(sys.argv[3:])))
        return 0

    if sys.argv[1] == "--ran":
        why = unrun(sys.argv[2])
        if why:
            print(why)
            sys.exit(1)
        sys.exit(0)

    if sys.argv[1] == "--judge-ready":
        # Asked before the lane spends a token on the candidate: a scenario with
        # judged criteria and no judge configured is a lane that would drive
        # twenty-one cases and then be unable to score them.
        for path in sys.argv[2:]:
            if not parse_scenario(path).get("judge", []):
                continue
            judge.configured()
            break
        return 0

    if sys.argv[1] == "--check":
        problems = check(parse_scenario(sys.argv[2]), sys.argv[3])
        for problem in problems:
            print(f"  {problem}")
        return 1 if problems else 0

    print(f"unknown mode {sys.argv[1]}", file=sys.stderr)
    return 2


if __name__ == "__main__":
    try:
        sys.exit(main())
    except judge.JudgeUnavailable as unavailable:
        # EXIT 2, NOT 1. The lane reads 1 as "this run failed the scenario" and
        # would record a red case for a judge that was never asked — the same
        # shape as the expired credential that once reported six broken use
        # cases. 2 is the harness stop scripts/e2e-llm.sh acts on.
        print(f"the judge could not be reached: {unavailable}", file=sys.stderr)
        sys.exit(2)
