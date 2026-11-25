#!/usr/bin/env python3
"""Read a scenario, and judge one run of it against what the assistant said.

Six things are checked per run, and all six must hold:

  must_call        Did it reach the tools it needed? An answer written from the
                   model's own memory, with Margince never asked, is a failure
                   however good the prose reads. Alternatives inside one entry
                   are an any-of group, as in must_mention — for the question
                   more than one engine answers honestly.
  must_mention     Does the answer contain what it has to contain? Each entry is
                   a regex, and alternatives inside one entry are an any-of
                   group.
  must_not_mention Does it avoid what it must avoid? This half matters more than
                   the first: a confident wrong answer is worse than no answer.
  must_not_call_with
                   Did it keep out of a call what must not be sent? Each entry
                   is `tool~regex`, searched in every call's arguments: the
                   file's text pasted into log_activity, which no answer shows.
  must_call_matching
                   The other direction: some call to `tool` carries what
                   `tool~regex` asks, when the target is named in free text
                   rather than one argument's exact value.
  figures_from_results
                   Is every money figure the run states one Margince returned in
                   this run, or worked out from one? `true` checks each amount
                   against the run's tool results, so an invented figure fails.
  judge            Does the answer do what this sentence says? Each entry is a
                   criterion in plain words, decided by a model (judge.py).

THE ANSWER IS SCORED, NOT THE PATH. A model may reach a right answer by more than
one road, and a case that pins one tool fails a correct answer for taking another.
Pin a tool only when the call is the outcome (a write the end state cannot show,
or a refusal to call); otherwise state what the answer must hold and let the model
choose. A run that calls nothing is scored the same way: its answer and end state
show whether it could be right.

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

What the run left in the database is not read here: a scenario's
`must_end_with` is endstate.py's, which the lane calls with the stack still up.

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

import collections
import datetime
import json
import math
import os
import re
import shlex
import statistics
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


def checked_path(path):
    """The real path of a file this lane was told to open, refusing anything
    outside the repo or the system temp directory.

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
    return real


def _open_checked(path):
    return open(checked_path(path), encoding="utf-8")


# Every key a scenario may carry, and its shape. A key outside this table is a
# typo that would switch its check off, so the reader refuses it.
_SCHEMA = {
    "name": "text", "title": "text", "prompt": "text", "writes": "text", "passport": "text",
    "setup": "text", "figures_from_results": "text", "runs": "count", "pass_at": "count",
    "criteria": "list", "must_call": "list", "may_call": "list", "must_call_with": "list",
    "must_not_call_with": "list", "must_call_matching": "list", "must_mention": "list",
    "must_not_mention": "list", "judge": "list", "must_end_with": "list", "counts_when_refused": "list",
    "elicitation": "list", "must_answer": "list", "mcp_only": "list", "expected_calls": "count",
}
# The checks an mcp_only entry may name: the ones that pin an MCP call's name or
# argument, which a REST door spells by route. Every other check reads the
# answer or the records and holds on both doors.
_DOOR_SCOPED = ("must_call", "must_call_with", "must_call_matching")
_REQUIRED = ("name", "prompt", "writes", "runs", "pass_at")
# The credentials scripts/e2e-llm.sh mints, by the `passport:` value that picks
# one; absent is the narrow one.
PASSPORTS = ("", "wide", "research", "rep", "readonly")


class ScenarioError(ValueError):
    """A scenario file the reader refuses, with every problem it found."""

    def __init__(self, path, problems):
        super().__init__(f"{os.path.basename(path)}: " + "; ".join(problems))
        self.problems = problems


def parse_scenario(path):
    """Read the subset of YAML the scenario files use, refusing what it cannot place."""
    data, problems = _read_scenario(path)
    if problems:
        raise ScenarioError(path, problems)
    return data


def _read_scenario(path):
    data, key, block, block_indent, seq, problems = {}, None, None, 0, None, []
    with _open_checked(path) as handle:
        text = handle.read()
    for number, raw in enumerate(text.splitlines(), start=1):
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
            problems.append(f"line {number}: a line the scenario reader cannot place: {line!r}")
            continue
        key, value = m.group(1), m.group(2).strip()
        if key not in _SCHEMA:
            problems.append(f"line {number}: unknown key {key}")
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
    return data, problems


def lint(path):
    """Every problem with one scenario file, found before a token is spent.

    The reader's own refusals, then each value against its shape: a list where
    a list belongs, a pattern that compiles, an entry the grader can read.
    """
    data, problems = _read_scenario(path)
    for key in _REQUIRED:
        if key not in data:
            problems.append(f"{key} is missing")
    for key, value in data.items():
        shape = _SCHEMA.get(key)
        if shape == "list" and not isinstance(value, list):
            problems.append(f"{key} must be a list, not {value!r}")
        elif shape == "count" and not (isinstance(value, int) and value > 0):
            problems.append(f"{key} must be a whole number above 0, not {value!r}")
    if problems:
        return problems
    if data["pass_at"] > data["runs"]:
        problems.append(f"pass_at {data['pass_at']} is more than runs {data['runs']}")
    # The lane's own reset and the write-declaration gate both read writes:, so
    # an absent or misspelled value would leave them comparing against nothing.
    if data["writes"] not in ("true", "false"):
        problems.append(f"writes must be true or false, not {data['writes']!r}")
    if data.get("passport", "") not in PASSPORTS:
        problems.append(f"passport must be absent or one of {', '.join(PASSPORTS[1:])}, not {data['passport']!r}")
    if data.get("figures_from_results", "true") != "true":
        problems.append(f"figures_from_results is {data['figures_from_results']!r}; the only value is true")
    if data.get("setup"):
        import casesetup

        if data["setup"] not in casesetup.STEPS:
            problems.append(f"setup {data['setup']!r} is no step in casesetup.py")
    return problems + _entry_problems(data) + _door_problems(data)


def _door_problems(data):
    """An mcp_only entry must repeat a door-scoped check of the same file exactly,
    or a typo in it would switch nothing off and read as if it had."""
    carried = {entry for key in _DOOR_SCOPED for entry in data.get(key, [])}
    return [f"mcp_only names {entry!r}, which no {', '.join(_DOOR_SCOPED)} entry of this scenario carries"
            for entry in data.get("mcp_only", []) if entry not in carried]


def for_door(scenario, path):
    """The scenario as the run's door grades it. A REST run is not held to the
    checks mcp_only names: they pin an MCP argument that no REST route takes."""
    if run_door(path) != "rest":
        return scenario
    skipped = set(scenario.get("mcp_only", []))
    return {key: [entry for entry in value if entry not in skipped] if key in _DOOR_SCOPED else value
            for key, value in scenario.items()}


def run_door(path):
    """The door the transcript's system line names; a run made over MCP names none."""
    init = _events(path, "init")
    return (init[0].get("door") if init else None) or "mcp"


# ELICITATION is what the user answers when the server asks in the chat:
# `<text the question holds> => accept|decline|cancel [field=value ...]`. Only a
# host that can answer reads it (E2E_LLM_VIA=sdk); a value is JSON when it
# parses as JSON, and a bare word otherwise.
_ELICITATION_ACTIONS = ("accept", "decline", "cancel")
_ELICITATION_FIELD = re.compile(r"[A-Za-z_][A-Za-z0-9_]*=\S+")


def _form_value(text):
    try:
        value = json.loads(text)
    except ValueError:
        return text
    # NaN and Infinity parse here, and no JSON reader the runner uses accepts them.
    if isinstance(value, float) and not math.isfinite(value):
        raise ValueError(f"{text} is not a finite number, so no form field holds it")
    if isinstance(value, (str, bool, int, float)) or (
        isinstance(value, list) and all(isinstance(item, str) for item in value)
    ):
        return value
    raise ValueError(f"{text} is not a value a form field holds")


def elicitation_replies(entries):
    """The scripted answers as the SDK runner reads them, in the scenario's order."""
    replies = []
    for entry in entries:
        match, _, answer = str(entry).partition("=>")
        action, *fields = answer.split() or [""]
        if not match.strip() or action not in _ELICITATION_ACTIONS or not all(
            _ELICITATION_FIELD.fullmatch(field) for field in fields
        ):
            raise ValueError(f"elicitation entry {entry!r} is not `<text> => accept|decline|cancel [field=value ...]`")
        reply = {"match": match.strip(), "action": action}
        if fields and reply["action"] != "accept":
            raise ValueError(f"elicitation entry {entry!r} gives fields to a {reply['action']}, which sends none")
        if reply["action"] == "accept":
            reply["content"] = {}
            for field in fields:
                key, _, value = field.partition("=")
                reply["content"][key] = _form_value(value)
        replies.append(reply)
    return replies


def _entry_problems(data):
    problems = []
    try:
        scripted = {(reply["match"], reply["action"]) for reply in elicitation_replies(data.get("elicitation", []))}
        for wanted in elicitation_replies(data.get("must_answer", [])):
            if (wanted["match"], wanted["action"]) not in scripted:
                problems.append(f"must_answer {wanted['match']!r} => {wanted['action']} is no elicitation entry")
    except ValueError as malformed:
        problems.append(str(malformed))
    for key in ("must_mention", "must_not_mention"):
        problems += [p for p in (_compiles(key, str(pattern)) for pattern in data.get(key, [])) if p]
    for entry in data.get("must_not_call_with", []):
        tool, sep, pattern = str(entry).partition("~")
        problems.append(_compiles("must_not_call_with", pattern) if sep and tool and pattern
                        else f"must_not_call_with entry {entry!r} is not tool~regex")
    for entry in data.get("must_call_matching", []):
        for alternative in str(entry).split(" || "):
            tool, sep, pattern = alternative.strip().partition("~")
            problems.append(_compiles("must_call_matching", pattern) if sep and tool and pattern
                            else f"must_call_matching entry {entry!r} is not tool~regex")
    for entry in data.get("must_call_with", []):
        for alternative in alternatives(entry):
            target, sep, _ = alternative.partition("=")
            tool, _, argument = target.partition(".")
            if not sep or not tool or not argument:
                problems.append(f"must_call_with entry {alternative!r} is not tool.argument=value")
    if data.get("must_end_with"):
        import endstate

        for entry in data["must_end_with"]:
            try:
                endstate.parse(entry)
            except ValueError as malformed:
                problems.append(str(malformed))
    return [p for p in problems if p]


def _compiles(key, pattern):
    try:
        re.compile(pattern)
    except re.error as err:
        return f"{key}: /{pattern}/ does not compile: {err}"
    return ""


def _scalar(text):
    if len(text) >= 2 and text[0] == text[-1] and text[0] in "\"'":
        text = text[1:-1]
    if re.fullmatch(r"-?\d+", text):
        return int(text)
    return text


# ROUTES are the served surface's routers, read from the table the backend
# derives (TestTheLanesRoutingTableIsPublished). A call to a router is recorded
# twice: as the router, and as the verb its action ran with the route field
# gone. A scenario may therefore name either, and one naming the verb keeps its
# meaning when the verb moves behind a router.
_ROUTES_FILE = os.path.join(os.path.dirname(os.path.abspath(__file__)), "routes.json")


def load_routes(path=_ROUTES_FILE):
    with open(path, encoding="utf-8") as fh:
        return json.load(fh).get("routers", {})


def routed(name, arguments, routes):
    """The verb a router call ran and its own arguments, or None for a plain call."""
    prefix, _, bare = name.rpartition("__")
    table = routes.get(bare)
    if not table or not table.get("field") or not isinstance(arguments, dict):
        return None
    verb = table.get("actions", {}).get(arguments.get(table["field"]))
    if not verb:
        return None
    rest = {k: v for k, v in arguments.items() if k != table["field"]}
    return (prefix + "__" + verb if prefix else verb), rest


def offered_verbs(offered, routes):
    """The offered names, and every verb an offered router fronts by its field."""
    out = list(offered)
    for name in offered:
        prefix, _, bare = name.rpartition("__")
        table = routes.get(bare, {})
        if not table.get("field"):
            continue
        for verb in table.get("actions", {}).values():
            out.append(prefix + "__" + verb if prefix else verb)
    return out


# One run as the grader reads it. `said` is every word the assistant wrote and
# `final` is the answer the user was left with; `calls` are every attempt and
# `worked` the calls whose result was not an error. A call with no result did
# not finish, so it is not in `worked`; `refused` are the others.
Run = collections.namedtuple("Run", "called said calls final worked refused")


def read_run(path):
    """Read a stream-json transcript: tool calls as `tool_use` blocks, prose as
    `text` blocks, each call's outcome as a `tool_result`, and the final answer
    as the terminal `result` event's text."""
    attempts, said, final, failed = [], [], "", {}
    with _open_checked(path) as handle:
        lines = handle.readlines()
    for line in lines:
        line = line.strip()
        if not line:
            continue
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            continue

        if event.get("type") == "result" and isinstance(event.get("result"), str):
            said.append(event["result"])
            final = event["result"]

        content = (event.get("message") or {}).get("content")
        if not isinstance(content, list):
            continue
        for block in content:
            if not isinstance(block, dict):
                continue
            if block.get("type") == "tool_use":
                attempts.append((block.get("id"), block.get("name", ""), block.get("input") or {}))
            elif block.get("type") == "tool_result":
                failed[block.get("tool_use_id")] = bool(block.get("is_error")) and not staged(block.get("content"))
            elif block.get("type") == "text":
                said.append(block.get("text", ""))

    routes = load_routes()
    called, calls, worked, refused = [], [], [], []
    for call_id, name, arguments in attempts:
        # Recorded twice on purpose, as the router and as its verb, so a
        # scenario may name either; nothing here counts calls.
        both = [(name, arguments)]
        verb = routed(name, arguments, routes)
        if verb:
            both.append(verb)
        called += [each for each, _ in both]
        calls += both
        if failed.get(call_id) is False:
            worked += both
        else:
            refused += both
    return Run(called, "\n".join(said), calls, final, worked, refused)


# A confirm-first write comes back as an error that staged it for a human, or
# that asks its user's yes in the conversation, and either is the call doing its
# job. The words are the server's own (stagedExplanation in
# backend/internal/modules/agents/explain.go, confirmInConversationLead in
# confirmcode.go); a result carries no other mark of it.
_STAGED = re.compile(
    r"confirm-first \(🟡\)|already approved this exact call|needs the user's yes in this conversation", re.IGNORECASE)


def staged(content):
    """Whether a tool result's content is a write staged for approval."""
    if isinstance(content, list):
        content = " ".join(str(part.get("text", "")) for part in content if isinstance(part, dict))
    return bool(_STAGED.search(str(content or "")))


def read_transcript(path):
    """(every tool name called, every word said, every call with its arguments)."""
    run = read_run(path)
    return run.called, run.said, run.calls


def read_tool_results(path):
    """The text of every tool result in a transcript, in order, one entry per
    text part, so a result's JSON is read whole."""
    results = []
    with _open_checked(path) as handle:
        lines = handle.readlines()
    for line in lines:
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            continue
        content = (event.get("message") or {}).get("content")
        if not isinstance(content, list):
            continue
        for block in content:
            if isinstance(block, dict) and block.get("type") == "tool_result":
                body = block.get("content")
                if isinstance(body, list):
                    results += [str(part.get("text", "")) for part in body if isinstance(part, dict)]
                else:
                    results.append(str(body or ""))
    return results


# A money figure as an answer writes it: a currency sign, code or word before or
# after a number. A space or a Swiss apostrophe groups thousands only before
# exactly three digits, so "€5 6 deals" is €5 and a count, not €56.
_GROUPING = "\u202f\u00a0 '\u2019"
_DIGITS = rf"\d{{1,3}}(?:[.,{_GROUPING}]\d{{3}})+(?:[.,]\d{{1,2}})?(?!\d)|\d+(?:[.,]\d+)?"
_SCALE = r"(?:\s?(?:[kK]|[mM]|bn|Mio\.?|million|thousand)\b)?"
_CURRENCY_WORD = r"(?:EUR|USD|GBP|CHF|euros?|dollars?|pounds?)\b"
_MONEY = re.compile(
    rf"(?:[€$£]|\b(?:EUR|USD|GBP|CHF)\s?)\s?({_DIGITS})({_SCALE})"
    rf"|\b({_DIGITS})({_SCALE})\s?(?:€|{_CURRENCY_WORD})"
)
_NUMBER = re.compile(r"\d[\d.,]*\d|\d")
_RATE = re.compile(r"(\d{1,3}(?:[.,]\d+)?)\s?%")
_SCALES = {"k": 1e3, "thousand": 1e3, "m": 1e6, "million": 1e6, "mio": 1e6, "mio.": 1e6, "bn": 1e9}


def _amount(digits, scale):
    """A written amount as (value, the step its writing rounds to)."""
    text = re.sub(f"[{_GROUPING}]", "", digits)
    # One separator followed by exactly one or two digits is a decimal mark;
    # every other separator groups thousands.
    decimal = re.search(r"[.,](\d{1,2})$", text)
    if decimal and not re.fullmatch(r"\d{1,3}([.,]\d{3})+", text):
        whole = re.sub(r"[.,]", "", text[: decimal.start()])
        value, step = float(f"{whole}.{decimal.group(1)}"), 10 ** -len(decimal.group(1))
    else:
        value, step = float(re.sub(r"[.,]", "", text)), 1.0
    factor = _SCALES.get(scale.strip().lower(), 1) if scale.strip() else 1
    return value * factor, step * factor


def _holds(value, step, held):
    near = lambda a, b: abs(a - b) <= step / 2 + 1e-9  # noqa: E731
    if any(near(value, number) for number in held):
        return True
    return any(near(value, a + b) or near(value, abs(a - b)) for i, a in enumerate(held) for b in held[i + 1:])


# A figure no result holds may still be the model's own correct arithmetic over
# figures that results do hold, such as a weighted total over several deals. The
# judge decides that against the amounts the run read; an invented or wrong
# figure still fails, in the final answer or anywhere the run said it.
WORKED_OUT = (
    "Check these money figures the run stated: {figures}. "
    "None of them appears as such in the data Margince returned in this run, "
    "so each must be read from that data or correctly worked out from it. "
    "A figure the final answer does not state is shown after the answer, in the line the run said it in. "
    "The data the run read held these amounts and rates, each named by its field and its record; "
    "a field ending in _minor is in the currency's minor units:\n{held}\n"
    "Answer yes if each of those figures is read from that data or correctly worked out from it, rounding aside. "
    "Answer no if any of them is invented, or wrong against that data, wherever in the run it was said."
)
# Where the judge's answer shows the lines a figure was said in before the end.
SAID_EARLIER = "\n\nSaid earlier in the run:\n"
# Numbers that are neither money nor a rate, such as counts, listed after the
# amounts. They are capped because a page of records carries many; no amount
# or rate is ever cut.
WORKED_OUT_OTHER_NUMBERS = 100
_RATE_FIELD = re.compile(r"probability|rate|percent|pct|share", re.IGNORECASE)
# The name shown for a record, on one line and short, so a record's name stays
# a label inside the criterion.
_RECORD_NAME_CHARS = 60

# A figure the run stated that no tool result holds, and the line it was said in.
Figure = collections.namedtuple("Figure", "text line")
# figure_problems' answer: the figures no result holds, and a problem for each.
Unheld = collections.namedtuple("Unheld", "figures problems")
# The judge's question about the figures no result holds, the answer it reads,
# and the problems it decides; criterion is None when every figure is held.
WorkedOut = collections.namedtuple("WorkedOut", "criterion answer problems")


def tools_by_run(paths, passing):
    """Each run's tools whose call worked, by bare name, and whether the run
    passed. The coverage page counts only a passing run's tools as driven."""
    runs = []
    for path in paths:
        if os.path.exists(path):
            tools = sorted({name.rsplit("__", 1)[-1] for name, _ in read_run(path).worked})
            runs.append({"passed": path in passing, "tools": tools})
    return runs


def recorded_runs(args, passed):
    """--record's transcripts and the passing ones among them, which follow
    --passing; refused when they disagree with the count that passed."""
    transcripts, passing = list(args), []
    if "--passing" in transcripts:
        at = transcripts.index("--passing")
        transcripts, passing = transcripts[:at], transcripts[at + 1:]
    if transcripts and (len(passing) != passed or not set(passing) <= set(transcripts)):
        raise ValueError(f"--passing names {len(passing)} of the transcripts and {passed} runs passed; "
                         "give every transcript, then --passing and each passing run's transcript once")
    return transcripts, passing


def worked_out_question(run, results):
    """The question the judge is asked about the figures no result holds, the
    answer it reads, and those figures' problems."""
    unheld = figure_problems(run.said, results)
    if not unheld.figures:
        return WorkedOut(None, run.final, [])
    figures = ", ".join(dict.fromkeys(figure.text for figure in unheld.figures))
    earlier = list(dict.fromkeys(figure.line for figure in unheld.figures if figure.text not in run.final))
    # The earlier lines are the model's own words, so they reach the judge
    # inside the answer's fence and never in the criterion.
    answer = run.final + (SAID_EARLIER + "\n".join(earlier) if earlier else "")
    held = "\n".join(f"- {entry}" for entry in labelled_amounts(results)) or "- nothing"
    return WorkedOut(WORKED_OUT.format(figures=figures, held=held), answer, unheld.problems)


def worked_out_problems(run, results):
    """The money figures the judge finds invented or wrong; none when every
    figure is a result's own or the judge finds each correctly worked out."""
    question = worked_out_question(run, results)
    if question.criterion is None:
        return []
    correct, reason = judge.verdict(question.criterion, question.answer)
    if correct:
        return []
    return question.problems + [f"{JUDGED_NO} the figures are worked out from the data\n      the judge's reason: {reason}"]


def labelled_amounts(results):
    """The amounts and rates the run's tool results hold, each named by its
    field and record, then other numbers; a result that is not JSON gives the
    money figures its text writes."""
    money, rates, other = [], [], []
    for text in results:
        try:
            document = json.loads(text)
        except json.JSONDecodeError:
            money += [f"a result's text: {match.group(0).strip()}" for match in _MONEY.finditer(text)]
            continue
        _label(document, "", "", (money, rates, other))
    other = list(dict.fromkeys(other))
    return list(dict.fromkeys(money)) + list(dict.fromkeys(rates)) + other[:WORKED_OUT_OTHER_NUMBERS]


def _label(value, path, record, found):
    money, rates, other = found
    if isinstance(value, list):
        for index, item in enumerate(value):
            _label(item, f"{path}[{index}]", record, found)
        return
    if not isinstance(value, dict):
        return
    name = next((value[key] for key in ("name", "title", "label") if isinstance(value.get(key), str)), None)
    if name:
        label = " ".join(name.replace('"', "'").split())[:_RECORD_NAME_CHARS]
        record = f' ("{label}")'
    for key, field in value.items():
        where = f"{path}.{key}" if path else str(key)
        if isinstance(field, (dict, list)):
            _label(field, where, record, found)
        elif isinstance(field, str):
            money += [f"{where}{record}: {match.group(0).strip()}" for match in _MONEY.finditer(field)]
        elif isinstance(field, bool) or not isinstance(field, (int, float)):
            continue
        elif key.endswith("_minor"):
            currency = value.get(key[: -len("_minor")] + "_currency") or value.get("currency")
            unit = f" {currency}" if isinstance(currency, str) else ""
            money.append(f"{where}{record}: {field}{unit}")
        else:
            (rates if _RATE_FIELD.search(key) else other).append(f"{where}{record}: {field}")


def figure_problems(said, results):
    """Each money figure the run stated that no tool result of the run holds.

    A result may carry an amount in minor units, so each number there counts
    also as a hundredth of itself; an answer may round, so a figure holds when
    a result is within half the step the answer wrote it to. A figure the answer
    worked out, the sum or the gap of two returned figures, holds too, and so does a
    returned amount at a rate the answer states and a result holds: a weighted value
    is an amount at its stage's probability.
    """
    held = []
    for text in results:
        for token in _NUMBER.findall(text):
            number = _amount(token, "")[0]
            held.append(number)
            # Only a bare integer can be minor units; "1,240,000" is written
            # for a reader, never as cents.
            if token.isdigit():
                held.append(number / 100)
    rates = {rate for rate in (_amount(token, "")[0] for token in _RATE.findall(said))
             if 0 < rate <= 100 and any(abs(rate - number) < 1e-9 for number in held)}
    held += [amount * rate / 100 for amount in list(held) for rate in rates]
    figures, problems = [], []
    for match in _MONEY.finditer(said):
        digits = match.group(1) or match.group(3)
        value, step = _amount(digits, match.group(2) or match.group(4) or "")
        if not _holds(value, step, held):
            text = match.group(0).strip()
            figures.append(Figure(text, _line_at(said, match.start())))
            problems.append(f"stated {text!r}, which no tool result of this run holds")
    return Unheld(figures, problems)


def _line_at(text, at):
    """The whole line of text that holds position at."""
    start = text.rfind("\n", 0, at) + 1
    end = text.find("\n", at)
    return text[start: end if end >= 0 else len(text)].strip()


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
        # A FIELD THAT IS NOT THERE IS NOT A ZERO. A renamed or dropped usage
        # field would otherwise be summed as nothing and reported as measured,
        # which is the same silent shortfall a missing transcript would be.
        if not all(isinstance(usage.get(field), int) for field in _USAGE_FIELDS):
            return None
        counted = {field: usage[field] for field in _USAGE_FIELDS}
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
        # The cost carries its OWN denominator. It is summed over the runs that
        # reported one, which is not always the runs that reported usage, and a
        # total quoted without saying how many runs are behind it is the shape
        # runs_measured already exists to refuse.
        "runs_costed": costed,
        "cost_usd": round(cost, 4) if costed else None,
    }


def recorded_stamp(name, clock=None):
    """What this verdict measured, from the stamps the lane wrote before the
    sweep (backend/internal/compose/mcpe2e), with the commit and the time it
    ran. None when the lane wrote none, as the offline harness does not.

    A scenario the stamps do not name is a stamp file from another tree, and a
    verdict filed under it would claim to have measured copy it never saw.

    graded_commit and graded_at are the checker's. Grading saved transcripts
    again sets E2E_LLM_GRADED_COMMIT and keeps the run's E2E_LLM_COMMIT and
    E2E_LLM_RAN_AT; a live grade leaves them equal.
    """
    path = os.environ.get("E2E_LLM_STAMPS")
    if not path:
        return None
    with _open_checked(path) as handle:
        stamps = json.load(handle)
    if name not in stamps:
        raise ValueError(f"the stamps at {path} carry no {name}: they were made for another tree")
    graded_at = (clock or (lambda: datetime.datetime.now(datetime.timezone.utc)))().strftime("%Y-%m-%dT%H:%M:%SZ")
    commit = os.environ.get("E2E_LLM_COMMIT", "")
    stamp = {**stamps[name], "commit": commit, "ran_at": os.environ.get("E2E_LLM_RAN_AT") or graded_at,
             "graded_commit": os.environ.get("E2E_LLM_GRADED_COMMIT") or commit, "graded_at": graded_at}
    # The judge that graded, which E2E_LLM_JUDGE_MODEL can make another than
    # the table's: a verdict judged by another model is not current.
    if stamp.get("judge"):
        stamp["judge"] = judge.pinned_model()
    return stamp


def settled(passed, made, runs, pass_at):
    """Whether the runs still to come can no longer change the verdict.

    A run past this point is paid for and cannot move PASS or FAIL, so the lane
    stops there. The verdict records the runs made, never the runs planned.
    """
    return passed >= pass_at or made - passed > runs - pass_at or made >= runs


def usage_line(expected, usage):
    """The report's one line for a scenario's cost.

    MEASURED OUT OF EXPECTED, always both. A sweep whose transcripts went
    missing would otherwise print a small total and read as a cheap run rather
    than an unread one — the same misreading as a coverage page whose count
    agreed while its sets did not.
    """
    measured = usage["runs_measured"]
    costed = usage["runs_costed"]
    cost = usage["cost_usd"]
    if cost is None:
        priced = "unreported"
    elif costed == measured:
        priced = f"${cost:.2f}"
    else:
        # A total over fewer runs than were measured says so where it is read,
        # not only in the JSON. Silently pricing part of a sweep as all of it is
        # this lane's own defect in miniature.
        priced = f"${cost:.2f} over {costed} of {measured}"
    short = "" if measured == expected else f"  <-- {expected - measured} run(s) unmeasured"
    return (
        f"  usage: {measured}/{expected} runs measured, "
        f"{usage['input_tokens']:,} in, {usage['output_tokens']:,} out, "
        f"{usage['cache_read_input_tokens']:,} cache-read, "
        f"cost {priced}{short}"
    )


# A call the CLI turned away before any server saw it: a bare tool name the
# model made up, which it then retries under the real one.
_NO_SUCH_TOOL = re.compile(r"No such tool available", re.IGNORECASE)


def read_effort(path):
    """How directly one run finished: its calls, refused calls, turns, tokens
    and wall time, or None when the run never reached its terminal `result`.

    An unfinished run is left out rather than counted short, because its few
    calls would read as the most direct run of all. A key the transcript does
    not carry is absent, never 0.
    """
    calls, refused, result = 0, 0, None
    with _open_checked(path) as handle:
        for line in handle:
            try:
                event = json.loads(line)
            except json.JSONDecodeError:
                continue
            if not isinstance(event, dict):
                continue
            if event.get("type") == "result":
                result = event
            for block in (event.get("message") or {}).get("content") or []:
                if not isinstance(block, dict):
                    continue
                if block.get("type") == "tool_use":
                    calls += 1
                elif block.get("type") == "tool_result" and _refused_result(block):
                    refused += 1
    if result is None:
        return None
    effort = {"calls": calls, "refused": refused}
    for field in ("num_turns", "duration_ms"):
        if isinstance(result.get(field), int):
            effort["turns" if field == "num_turns" else field] = result[field]
    usage = read_usage(path)
    if usage is not None:
        counted = usage[0]
        effort["prompt_tokens"] = sum(counted[field] for field in _USAGE_FIELDS if field != "output_tokens")
        effort["output_tokens"] = counted["output_tokens"]
    return effort


def _refused_result(block):
    if block.get("is_error") and not staged(block.get("content")):
        return True
    return bool(_NO_SUCH_TOOL.search(_tool_result_text(block)))


_EFFORT_METRICS = ("calls", "turns", "refused", "prompt_tokens", "output_tokens", "duration_ms")


def _spread(values):
    median = statistics.median(values)
    return {"median": int(median) if median == int(median) else median, "min": min(values), "max": max(values)}


def efficiency(paths, expected_calls=None):
    """The median and spread of each effort metric over the finished runs, or
    None when no run finished.

    A metric is reported only when every finished run carries it, so all of
    them share the one denominator `runs`. It is a score and never a verdict:
    nothing here moves PASS, FAIL or the exit code.
    """
    efforts = [effort for effort in map(read_effort, paths) if effort is not None]
    if not efforts:
        return None
    out = {"runs": len(efforts)}
    for metric in _EFFORT_METRICS:
        if all(metric in effort for effort in efforts):
            out[metric] = _spread([effort[metric] for effort in efforts])
    if expected_calls is not None:
        out["expected_calls"] = expected_calls
        out["slow"] = out["calls"]["median"] > expected_calls
    return out


def efficiency_line(measured):
    """The report's one line for how directly a scenario's runs finished."""
    if measured is None:
        return "  efficiency: no run finished, nothing measured"
    parts = [f"{measured['runs']} run(s) finished"]
    for metric in ("calls", "turns", "refused"):
        if metric in measured:
            spread = measured[metric]
            parts.append(f"{metric} {spread['median']} ({spread['min']}-{spread['max']})")
    if "duration_ms" in measured:
        parts.append(f"{measured['duration_ms']['median'] / 1000:.1f}s median")
    slow = ""
    if measured.get("slow"):
        slow = f"  <-- slow: median {measured['calls']['median']} calls, {measured['expected_calls']} expected"
    return "  efficiency: " + ", ".join(parts) + slow


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


# The CLI's own call to the model failing. Its terminal result opens with "API
# Error:", which a tool's refusal never does: that one arrives as a tool result,
# and the run goes on. A run cut off here was not scored by the model, whatever
# it had called before.
_MODEL_API_FAILED = re.compile(r"^(?:Failed to authenticate\.\s*)?API Error:", re.IGNORECASE)

# The server's answer when the client declined a question it put in the chat
# (backend/internal/modules/agents/inputrequired.go, errDeclinedInChat). Nobody
# sits at a lane run to answer one: a CLI that can show the form declines it by
# itself, so the run could not have done what the user asked.
_DECLINED_IN_CHAT = re.compile(r"the user said no in the chat", re.IGNORECASE)


def _tool_result_text(block):
    content = block.get("content")
    if isinstance(content, list):
        return " ".join(str(part.get("text", "")) for part in content if isinstance(part, dict))
    return str(content or "")


def unrun(path):
    """Why this run never happened, or "" when it did.

    A scenario failing every criterion and a run that never reached the model
    produce the same score, and only one of them is a finding about the product.
    That is not hypothetical: an expired key answered `401 API key is invalid`
    on all eighteen runs of a lane, every scenario was recorded as failing its
    criteria, and the verdict said six use cases were broken.

    ONLY three shapes. A transcript with no assistant turn never got as far as
    the model, one with no terminal `result` was cut off by its driver, and a
    terminal error naming a credential refusal never got past the door. Every
    OTHER `is_error` is a run that happened — the lane sets
    `--max-turns 20`, and exhausting it is a finding about the scenario, not
    about the harness. Excusing one of those would be this same defect inverted:
    a real answer thrown away as a harness fault, and the rest of the lane
    abandoned with it.
    """
    saw_assistant, called_tool, saw_result, declined = False, False, False, False
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
        if event.get("type") == "user":
            for block in (event.get("message") or {}).get("content") or []:
                if isinstance(block, dict) and block.get("type") == "tool_result" \
                        and _DECLINED_IN_CHAT.search(_tool_result_text(block)):
                    declined = True
        if event.get("type") == "result":
            saw_result = True
            if event.get("is_error"):
                failure = str(event.get("result") or "")
    if not saw_assistant:
        return "the transcript carries no assistant turn: the model was never reached"
    if not saw_result:
        return "the run never finished: no result event — the driver stopped mid-run"
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
    if _MODEL_API_FAILED.match(failure):
        return "the model's API stopped answering mid-run: " + failure
    # A no the scenario scripted (E2E_LLM_VIA=sdk) is the user's own answer and
    # the run's premise; only a question nobody scripted was declined for them.
    if declined and not any(scripted_by for scripted_by, _ in answered(path)):
        return ("the client declined a question the server asked in the chat, and no one in "
                "the lane can answer one, so the run could not do what was asked")
    return ""


def _events(path, *subtypes):
    """The transcript's system events of these subtypes, in order."""
    found = []
    with _open_checked(path) as lines:
        for line in lines:
            try:
                event = json.loads(line)
            except json.JSONDecodeError:
                continue
            if isinstance(event, dict) and event.get("type") == "system" and event.get("subtype") in subtypes:
                found.append(event)
    return found


def _inside_repo(path):
    real = os.path.realpath(path)
    return real == _REPO_ROOT or real.startswith(_REPO_ROOT + os.sep)


def unclean(path):
    """What the candidate loaded besides the lane's server, or "" when nothing.

    The lane measures a model that knows nothing of Margince, so a run that read
    this repository's rules, the operator's memory, plugins, skills or hooks is
    not scored. The CLI states each in its init line; a driver that writes its
    own init line states none of them and passes.
    """
    inits = _events(path, "init")
    if not inits:
        return "the transcript carries no init line, so what the candidate loaded is unknown"
    problems = []
    for event in inits:
        places = {event.get("cwd"), event.get("original_cwd"), *(event.get("additional_directories") or [])}
        problems += [f"it started inside the repository ({place})" for place in sorted(places, key=str)
                     if isinstance(place, str) and _inside_repo(place)]
        if event.get("memory_paths"):
            problems.append(f"it loaded the operator's memory ({json.dumps(event['memory_paths'])})")
        problems += [f"it loaded the plugin {plugin.get('name')}" for plugin in event.get("plugins") or ()
                     if isinstance(plugin, dict) and plugin.get("path") != "builtin"]
        if event.get("skills"):
            problems.append(f"it loaded skills: {', '.join(map(str, event['skills']))}")
    problems += [f"it ran the hook {event.get('hook_name')}" for event in _events(path, "hook_started")]
    return "; ".join(problems)


def offered_problem(scenario, path):
    """Why this run could not have satisfied must_call, or "" when it could.

    A required tool the server never offered fails must_call for a reason that
    is not the model's — a scope, a page the client did not follow, a server
    that attached nothing — so the lane stops rather than scoring it. The
    tools are the ones the transcript's system line says were offered.
    """
    scenario = for_door(scenario, path)
    init = _events(path, "init")
    if not init:
        return "the transcript carries no system line, so what was offered is unknown"
    offered = init[0].get("tools") or []
    if not offered:
        return "the server offered no tools at all"
    offered = offered_verbs(offered, load_routes())
    missing = [
        entry for entry in scenario.get("must_call", [])
        if not any(tool_matches(offered, alt) for alt in alternatives(entry))
    ]
    if missing:
        return f"a required tool was never offered to the model: {', '.join(map(str, missing))}"
    return ""


def tool_matches(called, want):
    """A tool is reached when any call names it.

    MCP tools arrive prefixed (mcp__margince__query_workspace), so the scenario
    names the bare tool and this matches the suffix — a scenario should not have
    to know the host's naming convention.
    """
    return any(name == want or name.endswith("__" + want) for name in called)


# ALTERNATIVES INSIDE ONE ENTRY ARE AN ANY-OF GROUP, which is what `|` already
# means in a must_mention entry, and it means the same thing here: this entry
# holds when ANY of its alternatives does.
#
# It exists because some questions have more than one honest engine. A count
# broken down by a dimension comes from run_report or run_analytics_query over
# the same population — so a scenario that named one of them would be measuring
# which engine the model happened to pick rather than whether the number came
# from a real aggregate. Separate entries
# cannot say that: they are ANDed, and would demand both.
#
# The separator is `|` with NO surrounding space, because a scenario's list item
# is read as one token by more than one reader. It follows that an argument
# VALUE carrying a literal `|` cannot use this construct; no scenario needs one,
# and the alternative — a second separator nobody recognises — would be worse.
def alternatives(entry):
    return [part.strip() for part in str(entry).split("|") if part.strip()]


def _called_with(calls, alternative):
    """Whether one `tool.argument=value` alternative holds, and what was seen.

    The value is compared against the argument rendered as JSON without quotes
    for a plain string, so both `report=activities-by-kind` and
    `group_by=["direction"]` are expressible.

    `=*` asks only that the argument was sent with a value, for an id the run
    mints itself: an approval_id a scenario cannot know but the redeeming call
    must carry, since the same call without it stages a second proposal.

    A malformed alternative RAISES rather than answering no: it is a fault in
    the scenario, and reading it as an argument the answer failed to send would
    file a broken entry as a finding about the product.
    """
    target, _, expected = alternative.partition("=")
    tool, _, argument = target.partition(".")
    if not tool or not argument:
        raise ValueError(f"must_call_with entry {alternative!r} is not tool.argument=value")
    seen = []
    for name, arguments in calls:
        if not tool_matches([name], tool):
            continue
        actual = arguments.get(argument)
        rendered = actual if isinstance(actual, str) else json.dumps(actual, separators=(",", ":"))
        seen.append(rendered)
        if expected == "*" and actual not in (None, "", [], {}):
            return True, seen
        if rendered == expected:
            return True, seen
    return False, seen


def _sent_problems(calls, entry):
    """A problem line per call whose arguments carry what `tool~regex` forbids.

    The arguments are searched as one JSON text, so the forbidden words are
    found whichever field the model put them in: a body, a summary, a subject.
    A malformed entry is a problem rather than a pass, like must_call_with's.
    """
    tool, sep, pattern = str(entry).partition("~")
    if not sep or not tool or not pattern:
        return [f"must_not_call_with entry {entry!r} is not tool~regex"]
    problems = []
    for name, arguments in calls:
        if not tool_matches([name], tool):
            continue
        found = re.search(pattern, json.dumps(arguments, ensure_ascii=False), re.IGNORECASE)
        if found:
            problems.append(f"called {tool} carrying {found.group(0)!r}, which /{pattern}/ forbids")
    return problems


def _unmatched_problem(calls, entry):
    """A problem line when no call carries what any `tool~regex` of the entry asks, or None.

    ` || ` joins alternatives, since `|` already means "or" inside each regex.
    """
    wanted = []
    for alternative in str(entry).split(" || "):
        tool, sep, pattern = alternative.strip().partition("~")
        if not sep or not tool or not pattern:
            return f"must_call_matching entry {entry!r} is not tool~regex"
        wanted.append((tool, pattern))
    for tool, pattern in wanted:
        for name, arguments in calls:
            if tool_matches([name], tool) and re.search(pattern, json.dumps(arguments, ensure_ascii=False), re.IGNORECASE):
                return None
    return "never called " + " or ".join(f"{tool} carrying anything matching /{pattern}/" for tool, pattern in wanted)


def answered(path):
    """(the entry that answered, the action) for each question the host answered."""
    found = []
    with _open_checked(path) as lines:
        for line in lines:
            try:
                event = json.loads(line)
            except json.JSONDecodeError:
                continue
            if isinstance(event, dict) and event.get("type") == "elicitation":
                found.append((event.get("scripted_by"), (event.get("reply") or {}).get("action")))
    return found


def check(scenario, transcript_path):
    """The problem lines of one run. The positive checks read the final answer
    and the calls that worked; the forbidding checks read every word and every
    attempt, because a wrong claim or a forbidden call counts wherever it is."""
    scenario = for_door(scenario, transcript_path)
    run = read_run(transcript_path)
    # A send the lane's stack cannot deliver is refused by design, and the
    # scenario grades what the answer says about it: there the call is the
    # outcome whether or not the server took it.
    calls = run.worked + [(name, arguments) for name, arguments in run.refused
                          if any(tool_matches([name], tool) for tool in scenario.get("counts_when_refused", []))]
    called, said = [name for name, _ in calls], run.said
    problems = []

    for entry in scenario.get("must_call", []):
        wanted = alternatives(entry)
        if not any(tool_matches(called, want) for want in wanted):
            names = sorted({name.rsplit("__", 1)[-1] for name, _ in run.refused})
            refused = f"; refused: {', '.join(names)}" if names else ""
            problems.append(
                f"never called {' or '.join(wanted)} — the answer was not drawn from Margince "
                f"(called: {', '.join(sorted(set(called))) or 'nothing'}{refused})"
            )

    # must_call_with entries are `tool.argument=value`, flat rather than nested
    # because parse_scenario reads a deliberately small YAML subset.
    for entry in scenario.get("must_call_with", []):
        wanted = alternatives(entry)
        held, seen = False, []
        try:
            for alternative in wanted:
                held, saw = _called_with(calls, alternative)
                seen.extend(saw)
                if held:
                    break
        except ValueError as malformed:
            problems.append(str(malformed))
            continue
        if not held:
            problems.append(
                f"never called {' or '.join(wanted)} — "
                f"saw {', '.join(repr(s) for s in seen) or 'no such call'}"
            )

    for entry in scenario.get("must_not_call_with", []):
        problems.extend(_sent_problems(run.calls, entry))

    for entry in scenario.get("must_call_matching", []):
        problem = _unmatched_problem(calls, entry)
        if problem:
            problems.append(problem)

    # MUST_ANSWER holds a case's premise: the question was asked and the host
    # gave the scripted answer. A run that never reached the question cannot
    # show what the answer does.
    asked = answered(transcript_path)
    for wanted in elicitation_replies(scenario.get("must_answer", [])):
        if (wanted["match"], wanted["action"]) not in asked:
            problems.append(f"the question holding {wanted['match']!r} was never answered {wanted['action']}")

    for pattern in scenario.get("must_mention", []):
        if not re.search(pattern, run.final, re.IGNORECASE):
            problems.append(f"never said anything matching /{pattern}/")

    for pattern in scenario.get("must_not_mention", []):
        found = re.search(pattern, said, re.IGNORECASE)
        if found:
            problems.append(f"said {found.group(0)!r}, which /{pattern}/ forbids")

    figures = scenario.get("figures_from_results")
    if figures == "true":
        problems.extend(worked_out_problems(run, read_tool_results(transcript_path)))
    elif figures is not None:
        problems.append(f"figures_from_results is {figures!r}; the only value is true")

    # THE SEMANTIC HALF, last because it is the only one that can cost money and
    # the only one that can raise. judge.verdict never answers "no" for a judge
    # it could not reach — that is JudgeUnavailable, and it propagates out of
    # here rather than being caught and scored, because a criterion nobody
    # decided is not a criterion the answer failed.
    for criterion in scenario.get("judge", []):
        held, reason = judge.verdict(criterion, run.final)
        if not held:
            problems.append(f"{JUDGED_NO} {criterion}\n      the judge's reason: {reason}")

    return problems


def main():
    if sys.argv[1] == "--field":
        scenario = parse_scenario(sys.argv[3])
        value = scenario.get(sys.argv[2], "")
        print(value if not isinstance(value, list) else "\n".join(map(str, value)))
        return 0

    if sys.argv[1] == "--candidate":
        # KEY=value lines for the lane to eval: which model, folder and wire a
        # candidate names on a route, refused before the stack boots.
        import candidates

        args = sys.argv[2:] + [""] * 5
        try:
            route = candidates.resolve(args[0], args[1], args[2], args[3], args[4])
        except candidates.RouteError as err:
            print(err, file=sys.stderr)
            return 1
        for key in ("model", "folder", "wire", "key_env", "effort"):
            print(f"{key.upper()}={shlex.quote(getattr(route, key))}")
        print(f"DRIVER={route.candidate}:{route.via}")
        return 0

    if sys.argv[1] == "--record":
        scenario = parse_scenario(sys.argv[2])
        runs = int(sys.argv[4])
        # The transcripts of THIS scenario's runs, so the committed verdict
        # carries what the answer cost as well as whether it held. They are
        # optional: the offline harness records verdicts with no transcripts at
        # all, and a lane that refused to record without them would make the
        # cheap path depend on the paid one. The ones after --passing are the
        # runs that passed.
        transcripts, passing = recorded_runs(sys.argv[5:], int(sys.argv[3]))
        usage = total_usage(transcripts) if transcripts else None
        record = {
            "scenario": scenario.get("name"),
            "criteria": scenario.get("criteria", []),
            "passed": int(sys.argv[3]),
            "runs": runs,
            "pass_at": scenario.get("pass_at"),
        }
        # How the number was measured, which the folder alone cannot say: a CLI
        # route and the comparable bridge can both have run one model.
        for field, variable in (
            ("driver", "E2E_LLM_DRIVER"),
            ("effort", "E2E_LLM_EFFORT"),
            ("system_prompt", "E2E_LLM_SYSTEM_PROMPT"),
            ("search", "E2E_LLM_SEARCH"),
            ("door", "E2E_LLM_DOOR"),
        ):
            if os.environ.get(variable):
                record[field] = os.environ[variable]
        if os.environ.get("E2E_LLM_SELF_JUDGED") in ("true", "false"):
            record["self_judged"] = os.environ["E2E_LLM_SELF_JUDGED"] == "true"
        stamp = recorded_stamp(record["scenario"])
        if stamp:
            record["stamp"] = stamp
        if usage is not None:
            # runs_measured rides WITH the totals into the committed record. A
            # later reader summing cost across scenarios can then tell a cheap
            # sweep from one whose transcripts it could not read, which the
            # totals alone cannot say.
            record["usage"] = usage
        if transcripts:
            record["tools_by_run"] = tools_by_run(transcripts, passing)
        measured = efficiency(transcripts, scenario.get("expected_calls")) if transcripts else None
        if measured is not None:
            if "slow" in measured:
                record["slow"] = measured.pop("slow")
            record["efficiency"] = measured
        print(json.dumps(record, indent=2))
        return 0

    if sys.argv[1] == "--settled":
        passed, made, runs, pass_at = (int(arg) for arg in sys.argv[2:6])
        return 0 if settled(passed, made, runs, pass_at) else 1

    if sys.argv[1] == "--usage":
        # One line of prose for the lane's report, formatted here because this
        # file owns what a usage total means — the shell would otherwise parse
        # the JSON and spell the same shape a second time.
        print(usage_line(int(sys.argv[2]), total_usage(sys.argv[3:])))
        return 0

    if sys.argv[1] == "--efficiency":
        scenario = parse_scenario(sys.argv[2])
        print(efficiency_line(efficiency(sys.argv[3:], scenario.get("expected_calls"))))
        return 0

    if sys.argv[1] == "--ran":
        why = unrun(sys.argv[2])
        if why:
            print(why)
            sys.exit(1)
        sys.exit(0)

    if sys.argv[1] == "--clean":
        problem = unclean(sys.argv[2])
        if problem:
            print(problem)
            return 1
        return 0

    if sys.argv[1] == "--offered":
        problem = offered_problem(parse_scenario(sys.argv[2]), sys.argv[3])
        if problem:
            print(problem)
            return 1
        return 0

    if sys.argv[1] == "--judge-ready":
        # Asked before the lane spends a token on the candidate: a scenario with
        # judged criteria and no judge configured is a lane that would drive
        # twenty-one cases and then be unable to score them.
        for path in sys.argv[2:]:
            if not parse_scenario(path).get("judge", []):
                continue
            judge.ready()
            break
        return 0

    if sys.argv[1] == "--lint":
        found = 0
        for path in sys.argv[2:]:
            for problem in lint(path):
                print(f"{os.path.basename(path)}: {problem}")
                found += 1
        return 1 if found else 0

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
    except Exception as crash:  # noqa: BLE001 — a grader that crashed scored nothing
        print(f"HARNESS: check.py {sys.argv[1]} crashed: {crash!r}", file=sys.stderr)
        sys.exit(2)
