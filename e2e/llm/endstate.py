#!/usr/bin/env python3
"""Read the world a run left behind, and hold it against the scenario.

Everything else in this lane grades what the assistant CALLED and SAID. Neither
reads the database, so an answer that reports a write which never landed scores
on its own sentence. A scenario's `must_end_with` names the end state instead,
one record field per entry, in the flat form parse_scenario reads:

    must_end_with:
      - company "Emsland Ventilbau GmbH" lifecycle=prospect

The record is found by its type and its WHOLE display name, then read back with
read_record; the field's value, rendered the way must_call_with renders an
argument, must equal what follows the `=`. It runs after the run and before the
lane restores the snapshot, through the lane's own passport. It is a reader
with fixed calls, never a model.

    endstate.py --scenario S --transcript T --mcp-url URL --token-env NAME

Exit 0 when every entry holds, 1 when one does not, 3 when the world could not
be read. The outcome is appended to the transcript as an `end_state` event, so
a reader of the kept records sees what the world said beside what the model did.
"""

import argparse
import collections
import json
import os
import re
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

import check  # noqa: E402  — the scenario reader and the path containment, shared
import mcpclient  # noqa: E402
import transcript  # noqa: E402

HARNESS = 3

Expectation = collections.namedtuple("Expectation", "record_type name field value")

_ENTRY = re.compile(r'^([a-z_]+) "([^"]+)" ([A-Za-z_][A-Za-z0-9_]*)=(.+)$')

# The field each type is NAMED by, which is what an entry quotes. Only the types
# a scenario asks about today; a new one is a line here and a name it can be
# found by.
_NAME_FIELD = {"company": "display_name"}

# list_records answers 50 a page. A seeded world is a few pages; a cursor still
# turning past this many is a reader going round in circles, not a big world.
_PAGE_LIMIT = 50
_MAX_PAGES = 40


class Unreadable(Exception):
    """The world could not be read: a harness fault, never something the model did."""


def parse(entry):
    """One must_end_with entry as an Expectation; ValueError when malformed."""
    match = _ENTRY.match(str(entry).strip())
    if not match:
        raise ValueError(f'must_end_with entry {entry!r} is not: type "Display Name" field=value')
    found = Expectation(*match.groups())
    if found.record_type not in _NAME_FIELD:
        raise ValueError(f"must_end_with entry {entry!r} names a type this reader cannot find by name")
    return found


def _data(session, tool, arguments):
    """The `data` of one reader call. A refusal is Unreadable: the lane's own
    admin passport reads every record, so a refusal is the reader's fault or
    the stack's, and scoring it would blame the model for it."""
    text, is_error = session.call(tool, arguments)
    if is_error:
        raise Unreadable(f"{tool} refused the end-state read: {text[:200]}")
    try:
        data = json.loads(text).get("data")
    except (ValueError, AttributeError) as err:
        raise Unreadable(f"{tool} answered in a shape the reader cannot read: {text[:200]}") from err
    if not isinstance(data, dict):
        raise Unreadable(f"{tool} answered with no data object: {text[:200]}")
    return data


def _matching(session, want):
    """Every record of the type whose name is exactly the entry's.

    A WHOLE ENUMERATION, not a search. search_records ranks, and a run that
    found nothing there could not tell an absent record from one ranked out of
    the page, so absence would be a guess. Every page of list_records is the
    whole set the passport can see, and a name missing from it is missing.
    """
    name_field = _NAME_FIELD[want.record_type]
    found, cursor = [], None
    for _ in range(_MAX_PAGES):
        arguments = {"record_type": want.record_type, "limit": _PAGE_LIMIT}
        if cursor:
            arguments["cursor"] = cursor
        page = _data(session, "list_records", arguments)
        records = page.get("records")
        if not isinstance(records, list):
            raise Unreadable(f"list_records answered with no records list: {str(page)[:200]}")
        found += [r for r in records if (r.get("fields") or {}).get(name_field) == want.name]
        cursor = page.get("next_cursor")
        if not cursor:
            return found
    raise Unreadable(f"list_records was still paging after {_MAX_PAGES} pages of {want.record_type}")


def _rendered(value):
    return value if isinstance(value, str) else json.dumps(value, separators=(",", ":"))


def end_state(session, entries):
    """(the entries that held, a problem line per entry that did not).

    A SCORED failure is only what the world itself says: the record is absent,
    its field holds another value, or two records answer to its name. Everything
    that leaves the reader unable to say — a refusal, a shape it cannot read, a
    field the read does not carry — raises Unreadable, because scoring it would
    file the reader's blindness as the model's mistake. A record the run hid
    from the lane's own seat reads as absent.
    """
    held, problems = [], []
    for entry in entries:
        want = parse(entry)
        matches = _matching(session, want)
        if not matches:
            problems.append(f'ended with no {want.record_type} named "{want.name}"')
            continue
        # The seed writes one record by this name, so a second can only be the
        # run's own create — a duplicate the model made, and scored as one.
        if len(matches) > 1:
            problems.append(f'ended with {len(matches)} {want.record_type} records named "{want.name}"')
            continue
        fields = _data(session, "read_record", {"record_type": want.record_type, "id": matches[0].get("id")}).get("fields")
        if not isinstance(fields, dict) or want.field not in fields:
            raise Unreadable(f'read_record on {want.record_type} "{want.name}" carries no field {want.field!r}')
        actual = _rendered(fields[want.field])
        if actual == want.value:
            held.append(entry)
        else:
            problems.append(
                f'ended with {want.record_type} "{want.name}" {want.field}={actual}, wanted {want.value}'
            )
    return held, problems


def main():
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--scenario", required=True)
    parser.add_argument("--transcript", required=True)
    parser.add_argument("--mcp-url", required=True)
    parser.add_argument("--token-env", required=True, help="the variable holding the MCP passport")
    args = parser.parse_args()
    entries = check.parse_scenario(args.scenario).get("must_end_with", [])
    if not entries:
        return 0
    try:
        session = mcpclient.Session(args.mcp_url, os.environ.get(args.token_env, ""))
        session.open()
        held, problems = end_state(session, entries)
    except (Unreadable, mcpclient.McpFault, ValueError) as fault:
        transcript.append_end_state(check.checked_path(args.transcript), fault=str(fault))
        print(f"  the end state could not be read: {fault}")
        return HARNESS
    transcript.append_end_state(check.checked_path(args.transcript), held=held, failed=problems)
    for problem in problems:
        print(f"  {problem}")
    return 1 if problems else 0


if __name__ == "__main__":
    # Any crash is the reader's, not the model's: exit 3 so the lane stops
    # instead of scoring a world nobody read.
    try:
        sys.exit(main())
    except Exception as crash:  # noqa: BLE001 — the backstop is the point
        print(f"  the end-state reader crashed: {crash!r}")
        sys.exit(HARNESS)
