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
argument, must equal what follows the `=`. An activity is named by its whole
subject ON a project, company or contact, and found on that record's timeline.
Three names are not fields: `document` holds when exactly one file on
the record's Documents tab carries that checksum under a name the glob matches,
`documents` when the tab holds exactly that many files, and `document_contract`
when the one file with that checksum is filed against the project's contract of
that whole title. All three are read with list_documents, so they see the tab a
human sees. More are not stored fields either:

    - company "Weserblick Logistik GmbH" records=0      how many records carry the name
    - company "Taunus Gerätebau GmbH" owner=Sofia Meier  the owner's name, not their id
    - deal "Rheinufer Netzmodernisierung" stage=Negotiation   the stage's name
    - company "Dom Digital GmbH" tagged=K5 Conference 2026    or untagged=, by read_tags
    - tag "Strategic Account" records=3   contacts, companies, deals and leads carrying it
    - team "Fleet Accounts" member=Sofia Meier   the colleague of that name is on the team
    - company "Aachener Metallwerke GmbH" contract=Wartungsvertrag 2027   a contract of that whole title
    - contact "Greta Lindqvist" own_draft=Preisworkshop   the user's own unsent draft there, by subject
    - automation "Stage-change notify" status=enabled   a rule by name, read_view's automations
    - me "admin@demo.test" timezone=Europe/Lisbon   the passport's own seat, as whoami answers it
    - me "admin@demo.test" saved_questions=1+   at least that many questions the seat saved
    - me "admin@demo.test" working_days=1,2,3,4   and working_hours=09:00-17:00, my_working_hours
    - me "admin@demo.test" unread_notices=0      the seat's unread notices
    - me "admin@demo.test" capture_excluded=news.test   a domain rule on the seat's capture
    - me "admin@demo.test" voice=*sentences*     the seat's Voice DNA preferences, by glob
    - me "admin@demo.test" forecast_call=45000000 EUR   the seat's newest call this quarter
    - deal "Rheinufer Netzmodernisierung" pipeline=Sales   the pipeline's name
    - deal "Rheinufer Netzmodernisierung" decision=*cfo*   one open sales decision, by glob
    - company "Reply Deutschland Betreuerwechsel" signal=*fund*   a signal on it, by glob
    - project "Leitstand Goslar" health=at_risk  the judged health state, or (none)
    - contact "Katrin Sommer" consent=marketing_email:granted   a purpose's state
    - contact "Mai Nguyen" stop=subject_request  a stop of that kind on the contact
    - contact "Katrin Sommer" data_subject_request=access   one open request names her
    - list "Messe Hannover 2026" member=Jens Falk   or nonmember=, a member by its name; members=12 counts
    - fx_rate "USD" rate=0.92                    the newest rate from that currency

An activity is also found on a company or a contact, by every page of the
activity list filtered to that record, and `records=N` counts the activities
of that whole subject there:

    - activity "Re: Lieferverzug" on company "Rhön Hydraulik AG" records=2

It runs after the run and before the lane restores the snapshot, through the
lane's own passport, and only through tools the server LISTS: a router is called
with its view, never by the hidden name it routes to, or the read would be
logged as hidden-tool traffic. It is a reader with fixed calls, never a model.
An import the run committed is still writing when the run ends, so the reader
first waits, bounded, for every run the transcript committed to stop running.

    endstate.py --scenario S --transcript T --mcp-url URL --token-env NAME

Exit 0 when every entry holds, 4 when one does not, 3 when the world could not
be read. A scored failure is not 1, because 1 is what Python itself exits with
when this file cannot even be imported, and that world was never read. The outcome is appended to the transcript as an `end_state` event, so
a reader of the kept records sees what the world said beside what the model did.
"""

import argparse
import collections
import decimal
import fnmatch
import json
import os
import re
import sys
import time

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
_NAME_FIELD = {"company": "display_name", "project": "name", "deal": "name",
               "contact": "full_name", "lead": "full_name", "deal_room": "title"}

# The types found ON a parent rather than by a list of their own, and the
# parents they are found on: an activity is named by its subject, which only
# its parent's timeline makes unique.
_FOUND_ON = {"activity": ("project", "company", "contact")}

# Not record fields: what list_documents answers about the record's files.
_DOCUMENT = "document"
_DOCUMENT_COUNT = "documents"
_DOCUMENT_CONTRACT = "document_contract"
_DOCUMENT_FIELDS = (_DOCUMENT, _DOCUMENT_COUNT, _DOCUMENT_CONTRACT)
_CHECKSUM = re.compile(r"^[0-9a-f]{64}$")

# Not record fields either: a count of records by name, the owner's and the
# stage's names rather than their ids, and the words read_tags lists on a record.
_RECORD_COUNT = "records"
_OWNER = "owner"
_STAGE = "stage"
_TAGGED, _UNTAGGED = "tagged", "untagged"
_TAG = "tag"
_TAGGABLE = ("company", "contact", "deal", "lead")
_TAG_COUNTS = ("contacts", "companies", "deals", "leads")
_TEAM, _MEMBER = "team", "member"
_CONTRACT = "contract"
_AUTOMATION, _ME = "automation", "me"
_SAVED_QUESTIONS = "saved_questions"
_WORKING_DAYS, _WORKING_HOURS = "working_days", "working_hours"
_PIPELINE = "pipeline"
_LIST, _NONMEMBER, _MEMBERS = "list", "nonmember", "members"
_FX_RATE = "fx_rate"
# Fields a me entry reads somewhere other than whoami.
_UNREAD_NOTICES, _CAPTURE_EXCLUDED, _VOICE, _FORECAST_CALL = (
    "unread_notices", "capture_excluded", "voice", "forecast_call")
_OWN_DRAFT = "own_draft"
_DRAFT_ANCHORS = ("company", "contact", "deal", "lead", "project")

# commit_import answers while the import is still writing. A four-row file
# lands in about a second; a run still going after this long is a stuck stack.
_IMPORT_WAIT_SECONDS = 90
_IMPORT_POLL_SECONDS = 1

# list_records answers 50 a page. A seeded world is a few pages; a cursor still
# turning past this many is a reader going round in circles, not a big world.
_PAGE_LIMIT = 50
_MAX_PAGES = 40


class _Memo:
    """The session, answering a repeated read from what it already answered.

    Every entry enumerates its record type whole, and the server rate-limits a
    passport: six entries on one type would page the same list six times. The
    world does not move while the reader reads it, so one answer per question
    is the same answer. read_import is the exception, because it is polled.
    """

    def __init__(self, session):
        self._session, self._answers = session, {}

    def call(self, tool, arguments):
        if tool == "read_import":
            return self._session.call(tool, arguments)
        key = (tool, json.dumps(arguments, sort_keys=True))
        if key not in self._answers:
            self._answers[key] = self._session.call(tool, arguments)
        return self._answers[key]


class Unreadable(Exception):
    """The world could not be read: a harness fault, never something the model did."""


def parse(entry):
    """One must_end_with entry as an Expectation; ValueError when malformed."""
    match = _ENTRY.match(str(entry).strip())
    if not match:
        raise ValueError(f'must_end_with entry {entry!r} is not: type "Display Name" field=value')
    record_type, name, parent_type, parent_name, field, value = match.groups()
    found = Expectation(record_type, name, field, value, parent_type, parent_name)
    if record_type == _TAG:
        if parent_type is not None or field != _RECORD_COUNT:
            raise ValueError(f"must_end_with entry {entry!r}: a tag is read only as records=<count>")
    elif record_type == _TEAM:
        if parent_type is not None or field != _MEMBER:
            raise ValueError(f"must_end_with entry {entry!r}: a team is read only as member=<colleague's name>")
    elif record_type == _LIST:
        if parent_type is not None or field not in (_MEMBER, _NONMEMBER, _MEMBERS) \
                or (field == _MEMBERS and not value.isdigit()):
            raise ValueError(f"must_end_with entry {entry!r}: a list is read only as member=, nonmember=<record name> "
                             "or members=<count>")
    elif record_type == _FX_RATE:
        if parent_type is not None or field != "rate" or not _is_decimal(value):
            raise ValueError(f"must_end_with entry {entry!r}: an fx_rate is read only as rate=<decimal>")
    elif record_type in (_AUTOMATION, _ME):
        if parent_type is not None or field in (_RECORD_COUNT, _OWN_DRAFT, _STAGE, _TAGGED, _UNTAGGED, _CONTRACT) \
                or field in _DOCUMENT_FIELDS or (record_type == _ME and field == _OWNER):
            raise ValueError(f"must_end_with entry {entry!r}: a {record_type} is read only by its own fields")
    elif parent_type is None and record_type not in _NAME_FIELD:
        raise ValueError(f"must_end_with entry {entry!r} names a type this reader cannot find by name")
    _special_wanted(entry, found)
    if parent_type is not None and parent_type not in _FOUND_ON.get(record_type, ()):
        raise ValueError(f"must_end_with entry {entry!r} names a {record_type} this reader cannot find on a {parent_type}")
    if field in (_DOCUMENT, _DOCUMENT_CONTRACT):
        _document_wanted(found)
    if field == _DOCUMENT_CONTRACT and parent_type != "project":
        raise ValueError("document_contract finds its contract on a project, so the entry needs: on project \"Name\"")
    if field == _DOCUMENT_COUNT and not value.isdigit():
        raise ValueError(f"documents wants a count of files, not {value!r}")
    return found


def _special_wanted(entry, want):
    """ValueError when a non-field name is asked of a record it cannot be read on."""
    if want.field == _RECORD_COUNT and not want.value.isdigit():
        raise ValueError(f"must_end_with entry {entry!r}: records wants a count of records")
    if want.field in (_TAGGED, _UNTAGGED) and (want.record_type not in _TAGGABLE or want.parent_type):
        raise ValueError(f"must_end_with entry {entry!r}: read_tags reads the words on a {', '.join(_TAGGABLE)}")
    if want.field in (_STAGE, _PIPELINE) and want.record_type != "deal":
        raise ValueError(f"must_end_with entry {entry!r}: only a deal has a {want.field}")
    if want.field in (_WORKING_DAYS, _WORKING_HOURS, *_ME_READERS) and want.record_type != _ME:
        raise ValueError(f"must_end_with entry {entry!r}: {want.field} is read on me")
    if want.field in _RECORD_READERS and (want.record_type != _RECORD_READERS[want.field][0] or want.parent_type):
        raise ValueError(f"must_end_with entry {entry!r}: {want.field} is read on a {_RECORD_READERS[want.field][0]} found by name")
    if want.field == _CONSENT and ":" not in want.value:
        raise ValueError(f"must_end_with entry {entry!r}: consent wants <purpose_key>:<state>")
    if want.field == _CONTRACT and (want.record_type != "company" or want.parent_type):
        raise ValueError(f"must_end_with entry {entry!r}: read_details lists the contracts of a company")
    if want.field == _OWNER and want.parent_type is not None:
        raise ValueError(f"must_end_with entry {entry!r}: owner is read on a record found by name")
    if want.field == _OWN_DRAFT and (want.record_type not in _DRAFT_ANCHORS or want.parent_type):
        raise ValueError(f"must_end_with entry {entry!r}: a draft is read on a {', '.join(_DRAFT_ANCHORS)}")
    if want.field == _SAVED_QUESTIONS:
        least = re.fullmatch(r"(\d+)\+", want.value)
        # One page holds the answer, so a count past it would need the cursor.
        if want.record_type != _ME or not least or not 1 <= int(least.group(1)) <= _PAGE_LIMIT:
            raise ValueError(f"must_end_with entry {entry!r}: saved_questions is read on me, as N+ with N from 1 to {_PAGE_LIMIT}")


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
        # Through its router: tools/call refuses a name tools/list does not offer.
        page = _data(session, "find_records", {"engine": "list", **arguments})
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
        raise Unreadable(f'read_project on project "{project}" carries no {name} section')
    if section.get("truncated"):
        raise Unreadable(f'read_project cut the {name} of project "{project}" short')
    return section["items"]


def _on_parent(session, want):
    """(the activities on the parent carrying the entry's subject, the parent
    project's 360 read or None, problem).

    A project's timeline is read whole in its overview, which also carries the
    contracts document_contract needs; a company's or contact's is every page
    of the activity list filtered to that record.
    """
    parent_id, problem = _named(session, want.parent_type, want.parent_name)
    if problem:
        return [], None, problem
    if want.parent_type == "project":
        view = _data(session, "read_project", {"view": "overview", "project_id": parent_id})
        found = [a for a in _section(view, "activities", want.parent_name) if a.get("subject") == want.name]
        return [{**a, "id": a.get("activity_id")} for a in found], view, ""
    filters = {"entity_type": want.parent_type, "entity_id": parent_id}
    found, cursor = [], None
    for _ in range(_MAX_PAGES):
        arguments = {"engine": "list", "record_type": "activity", "filters": filters, "limit": _PAGE_LIMIT}
        if cursor:
            arguments["cursor"] = cursor
        page = _data(session, "find_records", arguments)
        records = page.get("records")
        if not isinstance(records, list):
            raise Unreadable(f"list_records answered with no activity list on {want.parent_type} \"{want.parent_name}\"")
        found += [r for r in records if (r.get("fields") or {}).get("subject") == want.name]
        cursor = page.get("next_cursor")
        if not cursor:
            return found, None, ""
    raise Unreadable(f"list_records was still paging activities after {_MAX_PAGES} pages")


def _locate(session, want):
    """(record id, the parent project's 360 read or None, problem)."""
    if want.parent_type is None:
        record_id, problem = _named(session, want.record_type, want.name)
        return record_id, None, problem
    found, view, problem = _on_parent(session, want)
    if problem:
        return None, view, problem
    if not found:
        return None, view, f"ended with no {_where(want)}"
    if len(found) > 1:
        return None, view, f"ended with {len(found)} records for {_where(want)}"
    return found[0].get("id"), view, ""


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
    record = _data(session, "read_record", {"record_type": want.record_type, "id": record_id})
    fields = record.get("fields")
    if not isinstance(fields, dict):
        raise Unreadable(f"read_record on {_where(want)} carries no fields")
    # read_record leaves an empty field, and an unowned record's owner, out. A
    # run that emptied one has changed the world, so it is scored, not a fault.
    if want.field == _OWNER:
        owner = record.get("owner")
        actual = str(owner["name"]) if isinstance(owner, dict) and "name" in owner else "(none)"
    elif want.field == _STAGE:
        actual = _stage_name(session, fields.get("stage_id"), want)
    elif want.field == _PIPELINE:
        actual = _pipeline_name(session, fields.get("pipeline_id"), want)
    elif want.field in fields:
        actual = _rendered(fields[want.field])
    else:
        actual = "(empty)"
    if actual == want.value:
        return ""
    return f"ended with {_where(want)} {want.field}={actual}, wanted {want.value}"


def _stage_name(session, stage_id, want):
    """The name read_workspace gives the deal's stage: a stage id is minted per seed."""
    pipelines = _data(session, "read_workspace", {"topic": "pipelines"}).get("pipelines")
    if not isinstance(pipelines, list):
        raise Unreadable("read_workspace answered with no pipelines list")
    for pipeline in pipelines:
        for stage in pipeline.get("stages") or ():
            if stage.get("id") == stage_id:
                return str(stage.get("name"))
    raise Unreadable(f"read_workspace lists no stage {stage_id!r} for {_where(want)}")


def _pipeline_name(session, pipeline_id, want):
    """The name read_workspace gives the deal's pipeline: its id is minted per seed."""
    pipelines = _data(session, "read_workspace", {"topic": "pipelines"}).get("pipelines")
    if not isinstance(pipelines, list):
        raise Unreadable("read_workspace answered with no pipelines list")
    for pipeline in pipelines:
        if pipeline.get("id") == pipeline_id:
            return str(pipeline.get("name"))
    raise Unreadable(f"read_workspace lists no pipeline {pipeline_id!r} for {_where(want)}")


def _tags_problem(session, want, record_id):
    """The problem line for a tagged/untagged entry, or "" when it holds."""
    view = _data(session, "read_tags", {"mode": "record", "record_type": want.record_type, "record_id": record_id})
    tags = view.get("tags")
    # Withheld is the vocabulary hidden from this seat, not a record without
    # tags: reading it as none would score a world nobody saw.
    if view.get("withheld") or not isinstance(tags, list):
        raise Unreadable(f"read_tags withheld or did not list the tags on {_where(want)}")
    carries = want.value in [t.get("name") for t in tags]
    if carries == (want.field == _TAGGED):
        return ""
    names = ", ".join(str(t.get("name")) for t in tags) or "none"
    return f"ended with {_where(want)} {'' if carries else 'not '}tagged {want.value} (tags: {names})"


def _tag_count_problem(session, want):
    """The problem line for a tag "Name" records=N entry, or "" when it holds."""
    listing = _data(session, "read_tags", {"mode": "list", "include_archived": True})
    words = listing.get("tags")
    if not isinstance(words, list) or listing.get("truncated"):
        raise Unreadable("read_tags did not list the whole vocabulary")
    named = [w for w in words if w.get("name") == want.name]
    if len(named) != 1:
        return f'ended with {len(named)} tags named "{want.name}"'
    tag = _data(session, "read_tags", {"mode": "get", "tag_id": named[0].get("tag_id")})
    if not all(isinstance(tag.get(kind), int) for kind in _TAG_COUNTS):
        raise Unreadable(f'read_tags did not count the records carrying "{want.name}"')
    carrying = sum(tag[kind] for kind in _TAG_COUNTS)
    if carrying == int(want.value):
        return ""
    return f'ended with tag "{want.name}" on {carrying} records, wanted {want.value}'


def _team_problem(session, want):
    """The problem line for a team "Name" member=Colleague entry, or "" when it holds."""
    listing = _data(session, "read_view", {"view": "teams", "q": want.name, "limit": _PAGE_LIMIT}).get("data")
    if not isinstance(listing, list):
        raise Unreadable("read_view teams answered with no data list")
    named = [team for team in listing if team.get("name") == want.name]
    if len(named) != 1:
        return f'ended with {len(named)} teams named "{want.name}"'
    roster = _data(session, "read_workspace", {"topic": "colleagues", "q": want.value}).get("colleagues")
    if not isinstance(roster, list):
        raise Unreadable("read_workspace answered with no colleagues list")
    colleague = [c.get("user_id") for c in roster if c.get("display_name") == want.value]
    if len(colleague) != 1:
        raise Unreadable(f'the seed holds {len(colleague)} colleagues named "{want.value}"')
    member = _data(session, "read_workspace", {"topic": "member", "id": colleague[0]})
    if not isinstance(member.get("team_ids"), list):
        raise Unreadable(f'read_workspace member carries no team_ids for "{want.value}"')
    if named[0].get("id") in member["team_ids"]:
        return ""
    return f'ended with {want.value} not on team "{want.name}"'


def _contract_on_company_problem(session, want, record_id):
    """The problem line for a company "Name" contract=Title entry, or "" when it holds."""
    listing = _data(session, "read_details", {"action": "company_contracts", "id": record_id}).get("data")
    if not isinstance(listing, list):
        raise Unreadable("read_details company_contracts answered with no data list")
    found = sum(1 for contract in listing if contract.get("title") == want.value)
    if found == 1:
        return ""
    return f'ended with {found} contracts titled "{want.value}" on {_where(want)}'


def _automation_problem(session, want):
    """The problem line for an automation "Name" field=value entry, or "" when it holds."""
    rules, cursor = [], None
    for _ in range(_MAX_PAGES):
        arguments = {"view": "automations", "limit": _PAGE_LIMIT}
        if cursor:
            arguments["cursor"] = cursor
        page = _data(session, "read_view", arguments)
        if not isinstance(page.get("data"), list):
            raise Unreadable("read_view automations answered with no data list")
        rules += page["data"]
        cursor = (page.get("page") or {}).get("next_cursor")
        if not cursor:
            break
    else:
        raise Unreadable(f"read_view automations was still paging after {_MAX_PAGES} pages")
    named = [rule for rule in rules if rule.get("name") == want.name]
    if len(named) != 1:
        return f'ended with {len(named)} automations named "{want.name}"'
    if want.field == _OWNER:
        actual = _member_name(session, named[0].get("owner_id"))
    else:
        actual = _rendered(named[0][want.field]) if named[0].get(want.field) is not None else "(empty)"
    if actual == want.value:
        return ""
    return f"ended with {_where(want)} {want.field}={actual}, wanted {want.value}"


def _member_name(session, user_id):
    """A rule's owner by display name; "(none)" for the product's own rule, which has none."""
    if not user_id:
        return "(none)"
    member = _data(session, "read_workspace", {"topic": "member", "id": user_id})
    if not isinstance(member.get("display_name"), str):
        raise Unreadable(f"read_workspace member carries no display_name for {user_id}")
    return member["display_name"]


def _me_problem(session, want):
    """The problem line for a me "email" field=value entry, read through whoami."""
    seat = _data(session, "whoami", {})
    # The entry names the seat it means; a lane passport acting for another one
    # would score that seat's field as this one's.
    if seat.get("email") != want.name:
        raise Unreadable(f"the lane's passport acts for {seat.get('email')!r}, not {want.name!r}")
    if want.field == _SAVED_QUESTIONS:
        return _saved_questions_problem(session, want)
    if want.field in (_WORKING_DAYS, _WORKING_HOURS):
        actual = _working_hours(session, want.field)
    elif want.field in _ME_READERS:
        return _ME_READERS[want.field](session, want, seat)
    else:
        actual = _rendered(seat[want.field]) if seat.get(want.field) not in (None, "") else "(empty)"
    if actual == want.value:
        return ""
    return f'ended with {want.field}={actual} on the seat "{want.name}", wanted {want.value}'


def _working_hours(session, field):
    """The seat's own days as 1,2,3 or hours as 09:00-17:00. The fallback a seat
    that chose nothing is answered with reads like any other choice."""
    hours = _data(session, "read_workspace", {"topic": "my_working_hours"}).get("working_hours")
    if not isinstance(hours, dict) or not isinstance(hours.get("days"), list):
        raise Unreadable("read_workspace my_working_hours carries no working_hours")
    if field == _WORKING_DAYS:
        return ",".join(str(day) for day in hours["days"])
    return f"{hours.get('start_time')}-{hours.get('end_time')}"


def _saved_questions_problem(session, want):
    """The problem line for a saved_questions=N+ entry: at least N saved, since a
    run may save one question per figure."""
    least = re.fullmatch(r"(\d+)\+", want.value)
    saved = _data(session, "read_reports", {"kind": "report_runs", "limit": _PAGE_LIMIT}).get("data")
    if not isinstance(saved, list):
        raise Unreadable("read_reports report_runs carries no data list")
    if len(saved) >= int(least.group(1)):
        return ""
    return f'ended with {len(saved)} saved questions on the seat "{want.name}", wanted {want.value}'


def _own_draft_problem(session, want, record_id):
    """The problem line for an own_draft=<subject> entry, or "" when it holds.

    An agent's words on the anchor in place of the user's are a failure even
    under the user's subject, so the agent's mark is read beside the subject.
    """
    arguments = {"action": "mail_draft", "anchor_type": want.record_type, "anchor_id": record_id}
    text, is_error = session.call("read_details", arguments)
    if is_error and "not_found" in text:
        return f"ended with no unsent draft of the user's own on {_where(want)}"
    if is_error:
        raise Unreadable(f"read_details refused the end-state read: {text[:200]}")
    try:
        draft = json.loads(text).get("data")
    except (ValueError, AttributeError) as err:
        raise Unreadable(f"read_details answered in a shape the reader cannot read: {text[:200]}") from err
    if not isinstance(draft, dict) or not isinstance(draft.get("agent_drafted"), bool):
        raise Unreadable(f"read_details mail_draft carries no agent_drafted mark: {text[:200]}")
    if draft["agent_drafted"]:
        return f"ended with an agent's draft on {_where(want)} ({draft.get('subject')!r}), wanted the user's own"
    if draft.get("subject") == want.value:
        return ""
    return f"ended with the user's draft on {_where(want)} under {draft.get('subject')!r}, wanted {want.value!r}"


def _is_decimal(value):
    try:
        decimal.Decimal(value)
    except decimal.InvalidOperation:
        return False
    return True


def _pages(session, tool, arguments, rows_key, what):
    """Every row of a paged read whose page says next_cursor under `page`."""
    rows, cursor = [], None
    for _ in range(_MAX_PAGES):
        page = _data(session, tool, {**arguments, **({"cursor": cursor} if cursor else {})})
        if isinstance(page.get("result"), dict):
            page = page["result"]
        if not isinstance(page.get(rows_key), list):
            raise Unreadable(f"{what} answered with no {rows_key} list")
        rows += page[rows_key]
        cursor = (page.get("page") or {}).get("next_cursor")
        if not cursor:
            return rows
    raise Unreadable(f"{what} was still paging after {_MAX_PAGES} pages")


def _verdict(want, actual):
    if actual == want.value:
        return ""
    return f"ended with {_where(want)} {want.field}={actual}, wanted {want.value}"


def _list_problem(session, want):
    """The problem line for a list "Name" member=/nonmember=<record name> entry."""
    found = _data(session, "read_lists", {"mode": "find", "query": want.name}).get("result")
    lists = [row for row in (found or {}).get("data") or () if row.get("name") == want.name]
    if len(lists) != 1:
        return f'ended with {len(lists)} lists named "{want.name}"'
    members = _pages(session, "read_lists", {"mode": "members", "list_id": lists[0]["id"], "limit": _PAGE_LIMIT},
                     "data", "read_lists members")
    if want.field == _MEMBERS:
        return "" if len(members) == int(want.value) else \
            f'ended with {len(members)} members on list "{want.name}", wanted {want.value}'
    member_type = lists[0].get("entity_type")
    if member_type not in _NAME_FIELD:
        raise Unreadable(f'list "{want.name}" holds {member_type!r} records, which this reader cannot find by name')
    # Both directions need the one record of that name: a nonmember nobody can
    # find would pass whatever the list holds.
    member_id, problem = _named(session, member_type, want.value)
    if problem:
        return problem
    on = any(m.get("entity_id") == member_id for m in members)
    if on == (want.field == _MEMBER):
        return ""
    return f'ended with {want.value} {"not " if not on else ""}on list "{want.name}"'


def _fx_rate_problem(session, want):
    """The problem line for fx_rate "USD" rate=0.92: the newest rate from that currency."""
    rates = [r for r in _data(session, "read_workspace", {"topic": "fx_rates"}).get("data") or ()
             if r.get("from_currency") == want.name]
    if not rates:
        return f"ended with no {want.name} rate on the sheet"
    newest = max(rates, key=lambda r: str(r.get("effective_date")))
    # The sheet keeps one rate per currency and date (an upsert on both), so
    # two on the newest date is a sheet this reader does not understand.
    if sum(1 for r in rates if r.get("effective_date") == newest.get("effective_date")) > 1:
        raise Unreadable(f"the sheet holds two {want.name} rates for {newest.get('effective_date')}")
    if _is_decimal(str(newest.get("rate"))) and decimal.Decimal(str(newest["rate"])) == decimal.Decimal(want.value):
        return ""
    return f"ended with {want.name} rate={newest.get('rate')}, wanted {want.value}"


def _health_problem(session, want, record_id):
    """A project's current health state as its overview shows it; "(none)" unjudged."""
    health = _data(session, "read_project", {"view": "overview", "project_id": record_id}).get("health")
    if not isinstance(health, dict):
        raise Unreadable(f"read_project on {_where(want)} carries no health")
    return _verdict(want, str(health.get("state")) if health.get("judged") else "(none)")


def _consent_problem(session, want, record_id):
    """consent=<purpose_key>:<state>, read from the contact's consent panel."""
    purpose = want.value.partition(":")[0]
    states = _data(session, "read_details", {"action": "contact_consent", "id": record_id}).get("state")
    if not isinstance(states, list):
        raise Unreadable(f"read_details contact_consent on {_where(want)} carries no state list")
    held = [s for s in states if s.get("purpose_key") == purpose]
    if len(held) != 1:
        raise Unreadable(f'the workspace holds {len(held)} consent purposes keyed "{purpose}"')
    return _verdict(want, f"{purpose}:{held[0].get('state')}")


def _stop_problem(session, want, record_id):
    """stop=<kind>: the contact carries a stop of that kind."""
    stops = _data(session, "read_details", {"action": "contact_stops", "id": record_id}).get("stops")
    if not isinstance(stops, list):
        raise Unreadable(f"read_details contact_stops on {_where(want)} carries no stops list")
    if any(s.get("kind") == want.value for s in stops):
        return ""
    kinds = ", ".join(str(s.get("kind")) for s in stops) or "none"
    return f"ended with no {want.value} stop on {_where(want)} (stops: {kinds})"


def _subject_request_problem(session, want, record_id):
    """data_subject_request=<kind>: exactly one open request of that kind names the
    contact, by its id or by an address it carries, which is how a request waits
    until somebody resolves it."""
    fields = _data(session, "read_record", {"record_type": "contact", "id": record_id}).get("fields") or {}
    names = {record_id} | {str(e.get("email")).lower() for e in fields.get("emails") or () if isinstance(e, dict)}
    requests = _pages(session, "read_workspace", {"topic": "data_subject_requests", "limit": _PAGE_LIMIT},
                      "data", "read_workspace data_subject_requests")
    open_ones = [r for r in requests if str(r.get("subject_ref")).lower() in names and r.get("kind") == want.value
                 and r.get("status") in ("open", "in_progress")]
    if len(open_ones) == 1:
        return ""
    return f"ended with {len(open_ones)} open {want.value} requests for {_where(want)}, wanted 1"


def _decision_problem(session, want, record_id):
    """decision=<glob>: exactly one open sales decision on the deal reads as the glob."""
    decisions = _data(session, "read_reports", {"kind": "sales_decisions", "deal_id": record_id}).get("data")
    if not isinstance(decisions, list):
        raise Unreadable("read_reports sales_decisions carries no data list")
    matching = [d for d in decisions if d.get("deal_id") == record_id and d.get("status") == "open"
                and fnmatch.fnmatch(str(d.get("decision")).lower(), want.value.lower())]
    if len(matching) == 1:
        return ""
    return f"ended with {len(matching)} open decisions on {_where(want)} reading {want.value}, wanted 1"


def _signal_problem(session, want, record_id):
    """signal=<glob>: a signal on the company whose summary reads as the glob."""
    signals = _pages(session, "read_view", {"view": "signals", "limit": _PAGE_LIMIT}, "data", "read_view signals")
    on = [s for s in signals if record_id in (s.get("entity_id"), s.get("resolved_company_id"))]
    if any(fnmatch.fnmatch(str(s.get("summary")).lower(), want.value.lower()) for s in on):
        return ""
    return f"ended with {len(on)} signals on {_where(want)}, none reading {want.value}"


def _seat_verdict(want, actual):
    if actual == want.value:
        return ""
    return f'ended with {want.field}={actual} on the seat "{want.name}", wanted {want.value}'


def _unread_notices_problem(session, want, _seat):
    count = _data(session, "read_details", {"action": "notices"}).get("unread_count")
    if not isinstance(count, int):
        raise Unreadable("read_details notices carries no unread_count")
    return _seat_verdict(want, str(count))


def _capture_excluded_problem(session, want, _seat):
    """capture_excluded=<domain>: a domain rule on the seat's own capture."""
    rules = _data(session, "read_workspace", {"topic": "capture_exclusions"}).get("data")
    if not isinstance(rules, list):
        raise Unreadable("read_workspace capture_exclusions carries no data list")
    domains = [str(r.get("value")).lower() for r in rules if r.get("kind") == "domain"]
    if want.value.lower() in domains:
        return ""
    return f'ended with no capture exclusion for {want.value} on the seat "{want.name}" (domains: {", ".join(domains) or "none"})'


def _voice_problem(session, want, seat):
    """voice=<glob>: the seat's own preferences in its live Voice DNA read as the glob."""
    profiles = _data(session, "read_workspace", {"topic": "voice_profiles"}).get("data")
    if not isinstance(profiles, list):
        raise Unreadable("read_workspace voice_profiles carries no data list")
    own = [p for p in profiles if p.get("owner_id") == seat.get("acting_user_id") and not p.get("archived_at")]
    if any(fnmatch.fnmatch(str(p.get("personality_md")).lower(), want.value.lower()) for p in own):
        return ""
    return f'ended with {len(own)} voice profiles on the seat "{want.name}", none reading {want.value}'


def _forecast_call_problem(session, want, seat):
    """forecast_call=<minor> <currency>: the seat's newest call for this quarter,
    whichever scope it was filed under."""
    calls = []
    for scope in ({}, {"scope_kind": "owner", "scope_id": seat.get("acting_user_id")}):
        answered = _data(session, "read_view", {"view": "forecast_calls", **scope}).get("calls")
        if not isinstance(answered, list):
            raise Unreadable("read_view forecast_calls carries no calls list")
        calls += [c for c in answered if c.get("author_id") == seat.get("acting_user_id")]
    if not calls:
        return _seat_verdict(want, "(none)")
    newest = max(calls, key=lambda c: str(c.get("created_at")))
    return _seat_verdict(want, f"{newest.get('amount_minor')} {newest.get('currency')}")


_CONSENT = "consent"
# A field read off a record found by name, by a read of its own: (type, reader).
_RECORD_READERS = {
    "health": ("project", _health_problem),
    _CONSENT: ("contact", _consent_problem),
    "stop": ("contact", _stop_problem),
    "data_subject_request": ("contact", _subject_request_problem),
    "decision": ("deal", _decision_problem),
    "signal": ("company", _signal_problem),
}
_ME_READERS = {
    _UNREAD_NOTICES: _unread_notices_problem,
    _CAPTURE_EXCLUDED: _capture_excluded_problem,
    _VOICE: _voice_problem,
    _FORECAST_CALL: _forecast_call_problem,
}


def _bare(name):
    return str(name).rsplit("__", 1)[-1]


def settle_imports(session, calls, sleep=time.sleep, clock=time.monotonic):
    """Wait until no import the run committed is still writing.

    A run_id the server refuses to read is the model's own invention, not an
    import, so it is passed over: the records it never wrote read as absent.
    """
    runs = [args.get("run_id") for name, args in calls if _bare(name) == "commit_import" and args.get("run_id")]
    deadline = clock() + _IMPORT_WAIT_SECONDS
    for run_id in dict.fromkeys(runs):
        while True:
            text, is_error = session.call("read_import", {"view": "report", "run_id": run_id})
            if is_error:
                break
            try:
                state = (json.loads(text).get("data") or {}).get("state")
            except (ValueError, AttributeError) as err:
                raise Unreadable(f"read_import answered in a shape the reader cannot read: {text[:200]}") from err
            if state != "running":
                break
            if clock() >= deadline:
                raise Unreadable(f"import run {run_id} was still running {_IMPORT_WAIT_SECONDS}s after the run")
            sleep(_IMPORT_POLL_SECONDS)


def _problem(session, want):
    """The problem line for one entry, or "" when it holds."""
    if want.record_type == _TAG:
        return _tag_count_problem(session, want)
    if want.record_type == _TEAM:
        return _team_problem(session, want)
    if want.record_type == _AUTOMATION:
        return _automation_problem(session, want)
    if want.record_type == _ME:
        return _me_problem(session, want)
    if want.record_type == _LIST:
        return _list_problem(session, want)
    if want.record_type == _FX_RATE:
        return _fx_rate_problem(session, want)
    if want.field == _RECORD_COUNT:
        if want.parent_type is not None:
            on_parent, _, problem = _on_parent(session, want)
            if problem or len(on_parent) == int(want.value):
                return problem
            return f"ended with {len(on_parent)} records for {_where(want)}, wanted {want.value}"
        found = len(_matching(session, want.record_type, want.name))
        if found == int(want.value):
            return ""
        return f'ended with {found} {want.record_type} records named "{want.name}", wanted {want.value}'
    record_id, view, problem = _locate(session, want)
    if problem:
        return problem
    if want.field in _DOCUMENT_FIELDS:
        return _document_problem(session, want, record_id, view)
    if want.field in (_TAGGED, _UNTAGGED):
        return _tags_problem(session, want, record_id)
    if want.field == _CONTRACT:
        return _contract_on_company_problem(session, want, record_id)
    if want.field == _OWN_DRAFT:
        return _own_draft_problem(session, want, record_id)
    if want.field in _RECORD_READERS:
        return _RECORD_READERS[want.field][1](session, want, record_id)
    return _field_problem(session, want, record_id)


def end_state(session, entries, calls=()):
    """(the entries that held, a problem line per entry that did not).

    A SCORED failure is only what the world itself says: the record is absent,
    its field holds another value, or two records answer to its name. Everything
    that leaves the reader unable to say — a refusal, a shape it cannot read, a
    field the read does not carry — raises Unreadable, because scoring it would
    file the reader's blindness as the model's mistake. A record the run hid
    from the lane's own seat reads as absent.
    """
    wants = [parse(entry) for entry in entries]
    session = _Memo(session)
    settle_imports(session, calls)
    held, problems = [], []
    for entry, want in zip(entries, wants):
        problem = _problem(session, want)
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
        calls = check.read_transcript(check.checked_path(args.transcript))[2]
        held, problems = end_state(session, entries, calls)
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
