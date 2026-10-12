#!/usr/bin/env python3
"""Read the world a run left behind, and hold it against the scenario.

Everything else in this lane grades what the assistant CALLED and SAID. Neither
reads the database, so an answer that reports a write which never landed scores
on its own sentence. A scenario's `must_end_with` names the end state instead,
one record field per entry, in the flat form parse_scenario reads:

    must_end_with:
      - company "Emsland Ventilbau GmbH" lifecycle=prospect
      - company "Aachener Metallwerke GmbH" document=<sha256> *.md
      - company "Aachener Metallwerke GmbH" documents=1
      - activity "Signing" on project "Rollout" document_contract=<sha256> Service contract

The record is found by its type and its WHOLE display name, then read back with
read_record; the field's value, rendered the way must_call_with renders an
argument, must equal what follows the `=`. An activity has no list of its own,
so it is named by its whole subject ON a project, and found on that project's
timeline. Three names are not fields: `document` holds when exactly one file on
the record's Documents tab carries that checksum under a name the glob matches,
`documents` when the tab holds exactly that many files, and `document_contract`
when the one file with that checksum is filed against the project's contract of
that whole title. All three are read with list_documents, so they see the tab a
human sees. It runs after the run and before the
lane restores the snapshot, through the lane's own passport. It is a reader
with fixed calls, never a model.

    endstate.py --scenario S --transcript T --mcp-url URL --token-env NAME

Exit 0 when every entry holds, 4 when one does not, 3 when the world could not
be read. A scored failure is not 1, because 1 is what Python itself exits with
when this file cannot even be imported, and that world was never read. The outcome is appended to the transcript as an `end_state` event, so
a reader of the kept records sees what the world said beside what the model did.
"""

import argparse
import collections
import fnmatch
import json
import os
import re
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

import check  # noqa: E402  — the scenario reader and the path containment, shared
import mcpclient  # noqa: E402
import transcript  # noqa: E402

HARNESS = 3
FAILED = 4

Expectation = collections.namedtuple(
    "Expectation", "record_type name field value parent_type parent_name", defaults=(None, None)
)

_ENTRY = re.compile(r'^([a-z_]+) "([^"]+)" (?:on ([a-z_]+) "([^"]+)" )?([A-Za-z_][A-Za-z0-9_]*)=(.+)$')

# The field each type is NAMED by, which is what an entry quotes. Only the types
# a scenario asks about today; a new one is a line here and a name it can be
# found by.
_NAME_FIELD = {"company": "display_name", "project": "name"}

# The types found ON a parent rather than by a list of their own, and the
# parent's type: list_records has no activity, and read_project_360 carries the
# project's timeline whole.
_FOUND_ON = {"activity": "project"}

# Not record fields: what list_documents answers about the record's files.
_DOCUMENT = "document"
_DOCUMENT_COUNT = "documents"
_DOCUMENT_CONTRACT = "document_contract"
_DOCUMENT_FIELDS = (_DOCUMENT, _DOCUMENT_COUNT, _DOCUMENT_CONTRACT)
_CHECKSUM = re.compile(r"^[0-9a-f]{64}$")

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
    record_type, name, parent_type, parent_name, field, value = match.groups()
    found = Expectation(record_type, name, field, value, parent_type, parent_name)
    if parent_type is None and record_type not in _NAME_FIELD:
        raise ValueError(f"must_end_with entry {entry!r} names a type this reader cannot find by name")
    if parent_type is not None and _FOUND_ON.get(record_type) != parent_type:
        raise ValueError(f"must_end_with entry {entry!r} names a {record_type} this reader cannot find on a {parent_type}")
    if field in (_DOCUMENT, _DOCUMENT_CONTRACT):
        _document_wanted(found)
    if field == _DOCUMENT_CONTRACT and parent_type is None:
        raise ValueError("document_contract finds its contract on a project, so the entry needs: on project \"Name\"")
    if field == _DOCUMENT_COUNT and not value.isdigit():
        raise ValueError(f"documents wants a count of files, not {value!r}")
    return found


def _document_wanted(want):
    """(checksum, filename glob) of a document entry; ValueError when malformed.

    The checksum is what proves the bytes arrived whole: a model copying base64
    by hand can drop a character and still produce a file that decodes.
    """
    checksum, _, rest = want.value.partition(" ")
    if not _CHECKSUM.match(checksum):
        raise ValueError(f"{want.field} wants a lowercase sha256 hex checksum, not {checksum!r}")
    if not rest.strip():
        if want.field == _DOCUMENT_CONTRACT:
            raise ValueError("document_contract wants the contract's whole title after the checksum")
        raise ValueError("document wants a filename glob after the checksum, such as *.md")
    return checksum, rest.strip()


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


def _matching(session, record_type, name):
    """Every record of the type whose name is exactly the one given.

    A WHOLE ENUMERATION, not a search. search_records ranks, and a run that
    found nothing there could not tell an absent record from one ranked out of
    the page, so absence would be a guess. Every page of list_records is the
    whole set the passport can see, and a name missing from it is missing.
    """
    name_field = _NAME_FIELD[record_type]
    found, cursor = [], None
    for _ in range(_MAX_PAGES):
        arguments = {"record_type": record_type, "limit": _PAGE_LIMIT}
        if cursor:
            arguments["cursor"] = cursor
        page = _data(session, "list_records", arguments)
        records = page.get("records")
        if not isinstance(records, list):
            raise Unreadable(f"list_records answered with no records list: {str(page)[:200]}")
        found += [r for r in records if (r.get("fields") or {}).get(name_field) == name]
        cursor = page.get("next_cursor")
        if not cursor:
            return found
    raise Unreadable(f"list_records was still paging after {_MAX_PAGES} pages of {record_type}")


def _named(session, record_type, name):
    """(id, problem) of the one record of the type with that whole name."""
    matches = _matching(session, record_type, name)
    if not matches:
        return None, f'ended with no {record_type} named "{name}"'
    # The seed writes one record by this name, so a second can only be the
    # run's own create — a duplicate the model made, and scored as one.
    if len(matches) > 1:
        return None, f'ended with {len(matches)} {record_type} records named "{name}"'
    return matches[0].get("id"), ""


def _where(want):
    where = f'{want.record_type} "{want.name}"'
    if want.parent_type:
        where += f' on {want.parent_type} "{want.parent_name}"'
    return where


def _section(view, name, project):
    """The items of one section of a project's 360 read, refused unless whole:
    a section cut short cannot say that what it does not list is absent."""
    section = view.get(name)
    if not isinstance(section, dict) or not isinstance(section.get("items"), list):
        raise Unreadable(f'read_project_360 on project "{project}" carries no {name} section')
    if section.get("truncated"):
        raise Unreadable(f'read_project_360 cut the {name} of project "{project}" short')
    return section["items"]


def _locate(session, want):
    """(record id, the parent project's 360 read or None, problem).

    An activity is found by its subject on its project's timeline, the one
    activity read that is whole, and the same read carries the project's
    contracts for document_contract.
    """
    if want.parent_type is None:
        record_id, problem = _named(session, want.record_type, want.name)
        return record_id, None, problem
    project_id, problem = _named(session, want.parent_type, want.parent_name)
    if problem:
        return None, None, problem
    view = _data(session, "read_project_360", {"project_id": project_id})
    found = [a for a in _section(view, "activities", want.parent_name) if a.get("subject") == want.name]
    if not found:
        return None, view, f"ended with no {_where(want)}"
    if len(found) > 1:
        return None, view, f"ended with {len(found)} records for {_where(want)}"
    return found[0].get("activity_id"), view, ""


def _documents(session, record_type, record_id):
    """Every file on the record's Documents tab, every page of it."""
    found, cursor = [], None
    for _ in range(_MAX_PAGES):
        arguments = {"entity_type": record_type, "entity_id": record_id, "limit": _PAGE_LIMIT}
        if cursor:
            arguments["cursor"] = cursor
        page = _data(session, "list_documents", arguments)
        documents = page.get("documents")
        if not isinstance(documents, list):
            raise Unreadable(f"list_documents answered with no documents list: {str(page)[:200]}")
        found += documents
        cursor = page.get("next_cursor")
        if not cursor:
            return found
    raise Unreadable(f"list_documents was still paging after {_MAX_PAGES} pages")


def _document_problem(session, want, record_id, view):
    """The problem line for a document entry, or "" when it holds."""
    documents = _documents(session, want.record_type, record_id)
    where = _where(want)
    # A count, not a checksum, because a refused file can come back wrapped:
    # an SVG packed into a zip is new bytes under a new name.
    if want.field == _DOCUMENT_COUNT:
        if len(documents) == int(want.value):
            return ""
        names = ", ".join(str(d.get("filename")) for d in documents) or "none"
        return f"ended with {len(documents)} files on {where} ({names}), wanted {want.value}"
    checksum, rest = _document_wanted(want)
    carrying = [d for d in documents if d.get("checksum") == checksum]
    if not carrying:
        return f"ended with no file on {where} whose checksum is {checksum}"
    # Twice is the record carrying one file two times, which list_documents
    # exists to prevent, so it is scored like a duplicate record.
    if len(carrying) > 1:
        return f"ended with {len(carrying)} copies of the file on {where}"
    if want.field == _DOCUMENT_CONTRACT:
        return _contract_problem(carrying[0], rest, view, where, want.parent_name)
    name = str(carrying[0].get("filename"))
    if not fnmatch.fnmatchcase(name, rest):
        return f'ended with the file on {where} named "{name}", wanted {rest}'
    return ""


def _contract_problem(document, title, view, where, project):
    """The problem line when the file is not filed against the titled contract.

    The contract itself is not the run's to change: an agent reads contracts
    and never writes one, so a project without exactly one contract of that
    title is a world the seed did not build, and the reader stops on it.
    """
    contracts = _section(view, "contracts", project)
    titled = [c for c in contracts if c.get("title") == title]
    if len(titled) != 1:
        raise Unreadable(f'project "{project}" carries {len(titled)} contracts titled "{title}", not one')
    filed = document.get("contract_id")
    if filed == titled[0].get("contract_id"):
        return ""
    titles = {c.get("contract_id"): c.get("title") for c in contracts}
    against = f'"{titles[filed]}"' if filed in titles else (filed or "no contract")
    return f'ended with the file on {where} filed against {against}, wanted "{title}"'


def _rendered(value):
    return value if isinstance(value, str) else json.dumps(value, separators=(",", ":"))


def _field_problem(session, want, record_id):
    """The problem line for a record field entry, or "" when it holds."""
    fields = _data(session, "read_record", {"record_type": want.record_type, "id": record_id}).get("fields")
    if not isinstance(fields, dict) or want.field not in fields:
        raise Unreadable(f"read_record on {_where(want)} carries no field {want.field!r}")
    actual = _rendered(fields[want.field])
    if actual == want.value:
        return ""
    return f"ended with {_where(want)} {want.field}={actual}, wanted {want.value}"


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
        record_id, view, problem = _locate(session, want)
        if not problem and want.field in _DOCUMENT_FIELDS:
            problem = _document_problem(session, want, record_id, view)
        elif not problem:
            problem = _field_problem(session, want, record_id)
        if problem:
            problems.append(problem)
        else:
            held.append(entry)
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
    return FAILED if problems else 0


if __name__ == "__main__":
    # Any crash is the reader's, not the model's: exit 3 so the lane stops
    # instead of scoring a world nobody read.
    try:
        sys.exit(main())
    except Exception as crash:  # noqa: BLE001 — the backstop is the point
        print(f"  the end-state reader crashed: {crash!r}")
        sys.exit(HARNESS)
