"""The end-state reader, against a scripted world: what a run left behind is
scored, and a world the reader could not read is a harness fault, never a score.
"""

import contextlib
import io
import json
import os
import subprocess
import sys
import tempfile
import unittest
import unittest.mock

HERE = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
sys.path.insert(0, HERE)

import check  # noqa: E402
import endstate  # noqa: E402
import mcpclient  # noqa: E402
import transcript  # noqa: E402
from tests.fakes import FakeMcp  # noqa: E402

def _is_list_read(call):
    """A list read, as the reader sends it: find_records with engine list."""
    name, arguments = call
    return name == "find_records" and (arguments or {}).get("engine") == "list"


TOOLS = [{"name": name, "inputSchema": {"type": "object"}} for name in ("list_records", "read_record")]
EMSLAND = 'company "Emsland Ventilbau GmbH" lifecycle=prospect'


def world(companies, page_size=2):
    """list_records and read_record answering over {display_name: lifecycle}.

    Paged on purpose, smaller than the world: a reader that stopped at the first
    page would miss the record and report it absent.
    """
    rows = [(f"00000000-0000-0000-0000-{i:012d}", name, lifecycle)
            for i, (name, lifecycle) in enumerate(companies)]

    def list_records(arguments):
        start = int(arguments.get("cursor") or 0)
        page = rows[start:start + page_size]
        data = {"records": [{"record_type": "company", "id": row_id, "fields": {"display_name": name}}
                            for row_id, name, _ in page]}
        if start + page_size < len(rows):
            data["next_cursor"] = str(start + page_size)
        return json.dumps({"data": data}), False

    def read_record(arguments):
        for row_id, name, lifecycle in rows:
            if row_id == arguments.get("id"):
                fields = {"display_name": name, "lifecycle": lifecycle}
                return json.dumps({"data": {"record_type": "company", "id": row_id, "fields": fields}}), False
        return "not found", True

    return {"list_records": list_records, "read_record": read_record}


SEEDED = [("Kieler Pumpenwerk GmbH", "prospect"), ("Rhön Hydraulik AG", "prospect"),
          ("Emsland Werke", "customer"), ("Emsland Ventilbau GmbH", "prospect")]


class EndStateTest(unittest.TestCase):
    def read(self, replies, entries=(EMSLAND,)):
        with FakeMcp(TOOLS, replies=replies) as server:
            session = mcpclient.Session(server.url, "tok")
            session.open()
            return endstate.end_state(session, list(entries))

    def test_a_world_that_holds_the_expectation_passes(self):
        held, problems = self.read(world(SEEDED))
        self.assertEqual((held, problems), ([EMSLAND], []))

    def test_a_record_left_at_another_value_is_a_scored_failure(self):
        changed = SEEDED[:3] + [("Emsland Ventilbau GmbH", "target")]
        held, problems = self.read(world(changed))
        self.assertEqual(held, [])
        self.assertEqual(problems, ['ended with company "Emsland Ventilbau GmbH" lifecycle=target, wanted prospect'])

    def test_a_record_that_is_not_there_is_a_scored_failure(self):
        held, problems = self.read(world(SEEDED[:3]))
        self.assertEqual(problems, ['ended with no company named "Emsland Ventilbau GmbH"'])

    def test_a_name_is_matched_whole_not_by_prefix(self):
        _, problems = self.read(world(SEEDED[:3]), ['company "Emsland" lifecycle=customer'])
        self.assertEqual(problems, ['ended with no company named "Emsland"'])

    def test_two_records_of_one_name_are_a_scored_failure(self):
        held, problems = self.read(world(SEEDED + [("Emsland Ventilbau GmbH", "target")]))
        self.assertEqual(held, [])
        self.assertEqual(problems, ['ended with 2 company records named "Emsland Ventilbau GmbH"'])

    def test_a_refused_read_is_a_harness_fault(self):
        replies = {**world(SEEDED), "read_record": ("permission denied", True)}
        with self.assertRaisesRegex(endstate.Unreadable, "read_record refused"):
            self.read(replies)

    def test_a_field_the_read_leaves_out_is_scored_as_empty(self):
        _, problems = self.read(world(SEEDED), ['company "Emsland Ventilbau GmbH" tier=gold'])
        self.assertEqual(problems, ['ended with company "Emsland Ventilbau GmbH" tier=(empty), wanted gold'])

    def test_a_transport_error_is_a_harness_fault(self):
        with FakeMcp(TOOLS, status=500) as server:
            session = mcpclient.Session(server.url, "tok")
            with self.assertRaises(mcpclient.McpFault):
                endstate.end_state(session, [EMSLAND])


NOTES = "a" * 64
FILED = f'company "Emsland Ventilbau GmbH" document={NOTES} *.md'
ONLY_ONE = 'company "Emsland Ventilbau GmbH" documents=1'


def documents_on(files, page_size=1):
    """list_documents over [(filename, checksum)], filed on every company alike.

    Paged one to a page, so a reader that stopped at the first page would miss
    every file but the newest.
    """
    def list_documents(arguments):
        start = int(arguments.get("cursor") or 0)
        page = [{"filename": name, "checksum": checksum} for name, checksum in files[start:start + page_size]]
        data = {"documents": page}
        if start + page_size < len(files):
            data["next_cursor"] = str(start + page_size)
        return json.dumps({"data": data}), False

    return {**world(SEEDED), "list_documents": list_documents}


class DocumentTest(unittest.TestCase):
    def read(self, files, entries=(FILED, ONLY_ONE)):
        tools = TOOLS + [{"name": "list_documents", "inputSchema": {"type": "object"}}]
        with FakeMcp(tools, replies=documents_on(files)) as server:
            session = mcpclient.Session(server.url, "tok")
            session.open()
            return endstate.end_state(session, list(entries))

    def test_the_file_on_the_record_under_its_kind_of_name_holds(self):
        held, problems = self.read([("visit-notes.md", NOTES)])
        self.assertEqual((held, problems), ([FILED, ONLY_ONE], []))

    def test_the_file_is_found_past_the_first_page(self):
        held, problems = self.read([("logo.png", "c" * 64), ("visit-notes.md", NOTES)], [FILED])
        self.assertEqual((held, problems), ([FILED], []))

    def test_a_file_whose_bytes_changed_on_the_way_is_absent(self):
        _, problems = self.read([("visit-notes.md", "d" * 64)], [FILED])
        self.assertEqual(problems, [f'ended with no file on company "Emsland Ventilbau GmbH" whose checksum is {NOTES}'])

    def test_the_right_bytes_under_another_kind_of_name_fail(self):
        _, problems = self.read([("visit-notes.txt", NOTES)], [FILED])
        self.assertEqual(problems, ['ended with the file on company "Emsland Ventilbau GmbH" named "visit-notes.txt", wanted *.md'])

    def test_one_file_attached_twice_fails(self):
        _, problems = self.read([("visit-notes.md", NOTES), ("visit-notes.md", NOTES)], [FILED])
        self.assertEqual(problems, ['ended with 2 copies of the file on company "Emsland Ventilbau GmbH"'])

    def test_a_refused_file_kept_in_another_wrapper_is_counted(self):
        _, problems = self.read([("am-logo.svg.zip", "e" * 64), ("visit-notes.md", NOTES)], [ONLY_ONE])
        self.assertEqual(problems, ['ended with 2 files on company "Emsland Ventilbau GmbH" '
                                    '(am-logo.svg.zip, visit-notes.md), wanted 1'])

    def test_a_listing_with_no_documents_is_a_harness_fault(self):
        replies = {**documents_on([]), "list_documents": (json.dumps({"data": {}}), False)}
        tools = TOOLS + [{"name": "list_documents", "inputSchema": {"type": "object"}}]
        with FakeMcp(tools, replies=replies) as server:
            session = mcpclient.Session(server.url, "tok")
            session.open()
            with self.assertRaisesRegex(endstate.Unreadable, "no documents list"):
                endstate.end_state(session, [FILED])


SIGNED = "b" * 64
SERVICE = "00000000-0000-0000-0000-00000000c001"
OTHER = "00000000-0000-0000-0000-00000000c002"
MEETING = "00000000-0000-0000-0000-00000000a001"
ON_MEETING = 'activity "Signing" on project "Rollout" '
FILED_ON_MEETING = ON_MEETING + f"document={SIGNED} *.pdf"
AGAINST_SERVICE = ON_MEETING + f"document_contract={SIGNED} Service contract"


def project_world(files, timeline=None, contracts=None, truncated=False):
    """One project, its 360 read and the files on the activities it lists.

    files maps an activity id to [(filename, checksum, contract_id)], so a file
    on the wrong activity is on the record, just not the one asked about.
    """
    timeline = [("Signing", MEETING)] if timeline is None else timeline
    contracts = [("Service contract", SERVICE), ("Old contract", OTHER)] if contracts is None else contracts

    def list_records(_arguments):
        rows = [{"record_type": "project", "id": "p1", "fields": {"name": "Rollout"}}]
        return json.dumps({"data": {"records": rows}}), False

    def read_project(arguments):
        # The router, with its view: read_project_360 is the hidden name it
        # routes to, and a call by that name is logged as hidden-tool traffic.
        if arguments.get("project_id") != "p1" or arguments.get("view") != "overview":
            return "not found", True
        view = {
            "activities": {"items": [{"activity_id": i, "subject": s} for s, i in timeline], "truncated": truncated},
            "contracts": {"items": [{"contract_id": i, "title": t} for t, i in contracts], "truncated": False},
        }
        return json.dumps({"data": view}), False

    def list_documents(arguments):
        rows = [{"filename": n, "checksum": c, "contract_id": k} for n, c, k in files.get(arguments.get("entity_id"), [])]
        return json.dumps({"data": {"documents": rows}}), False

    return {"list_records": list_records, "read_project": read_project, "list_documents": list_documents}


class FoundOnAProjectTest(unittest.TestCase):
    def read(self, replies, entries=(FILED_ON_MEETING, AGAINST_SERVICE)):
        tools = [{"name": n, "inputSchema": {"type": "object"}} for n in replies]
        with FakeMcp(tools, replies=replies) as server:
            session = mcpclient.Session(server.url, "tok")
            session.open()
            return endstate.end_state(session, list(entries))

    def test_the_file_on_the_meeting_against_the_contract_holds(self):
        held, problems = self.read(project_world({MEETING: [("signed.pdf", SIGNED, SERVICE)]}))
        self.assertEqual((held, problems), ([FILED_ON_MEETING, AGAINST_SERVICE], []))

    def test_a_file_against_another_contract_names_it(self):
        _, problems = self.read(project_world({MEETING: [("signed.pdf", SIGNED, OTHER)]}), [AGAINST_SERVICE])
        self.assertEqual(problems, ['ended with the file on activity "Signing" on project "Rollout" '
                                    'filed against "Old contract", wanted "Service contract"'])

    def test_a_file_against_no_contract_fails(self):
        _, problems = self.read(project_world({MEETING: [("signed.pdf", SIGNED, None)]}), [AGAINST_SERVICE])
        self.assertEqual(problems, ['ended with the file on activity "Signing" on project "Rollout" '
                                    'filed against no contract, wanted "Service contract"'])

    def test_a_file_on_another_activity_is_not_on_the_meeting(self):
        timeline = [("Signing", MEETING), ("Kickoff", "a2")]
        _, problems = self.read(project_world({"a2": [("signed.pdf", SIGNED, SERVICE)]}, timeline), [FILED_ON_MEETING])
        self.assertEqual(problems, ['ended with no file on activity "Signing" on project "Rollout" '
                                    f'whose checksum is {SIGNED}'])

    def test_a_meeting_not_on_the_timeline_is_a_scored_failure(self):
        _, problems = self.read(project_world({}, timeline=[("Kickoff", "a2")]), [FILED_ON_MEETING])
        self.assertEqual(problems, ['ended with no activity "Signing" on project "Rollout"'])

    def test_a_timeline_cut_short_is_a_harness_fault(self):
        with self.assertRaisesRegex(endstate.Unreadable, "cut the activities"):
            self.read(project_world({}, truncated=True), [FILED_ON_MEETING])

    def test_a_contract_the_seed_did_not_write_is_a_harness_fault(self):
        replies = project_world({MEETING: [("signed.pdf", SIGNED, SERVICE)]}, contracts=[("Old contract", OTHER)])
        with self.assertRaisesRegex(endstate.Unreadable, '0 contracts titled "Service contract"'):
            self.read(replies, [AGAINST_SERVICE])


QUALIFIED, NEGOTIATION = "s-qualified", "s-negotiation"


def crm_world(records, tags=None, words=None, runs=None, timeline=None):
    """A workspace over [(record_type, id, name, fields, owner_name)].

    timeline maps a record id to the subjects of the activities filed on it,
    listed one to a page so a reader that stopped at the first page misses one.

    tags maps a record id to the words read_tags lists on it; words maps a word
    to (tag_id, {kind: count}) for read_tags list and get; runs maps an import
    run id to the states read_import answers in turn, the last one repeating.
    """
    tags, words, runs = tags or {}, words or {}, {k: list(v) for k, v in (runs or {}).items()}
    timeline = timeline or {}
    name_field = {"company": "display_name", "deal": "name", "contact": "full_name", "lead": "full_name"}

    def ok(data):
        return json.dumps({"data": data}), False

    def list_records(arguments):
        if arguments.get("record_type") == "activity":
            return activities(arguments)
        rows = [{"record_type": t, "id": i, "fields": {name_field[t]: n}} for t, i, n, _, _ in records
                if t == arguments.get("record_type")]
        return ok({"records": rows})

    def activities(arguments):
        filters = arguments.get("filters") or {}
        parent = next((i for t, i, _, _, _ in records
                       if i == filters.get("entity_id") and t == filters.get("entity_type")), None)
        subjects = timeline.get(parent, [])
        start = int(arguments.get("cursor") or 0)
        page = {"records": [{"record_type": "activity", "id": f"a{start}", "fields": {"subject": s}}
                            for s in subjects[start:start + 1]]}
        if start + 1 < len(subjects):
            page["next_cursor"] = str(start + 1)
        return ok(page)

    def read_record(arguments):
        for t, i, n, fields, owner in records:
            if i == arguments.get("id") and t == arguments.get("record_type"):
                record = {"record_type": t, "id": i, "fields": {name_field[t]: n, **fields}}
                if owner is not None:
                    record["owner"] = {"id": "u-" + owner, "name": owner, "is_you": False}
                return ok(record)
        return "not found", True

    def read_workspace(arguments):
        if arguments.get("topic") != "pipelines":
            return "unknown topic", True
        stages = [{"id": QUALIFIED, "name": "Qualified"}, {"id": NEGOTIATION, "name": "Negotiation"}]
        return ok({"pipelines": [{"id": "p", "name": "Sales", "stages": stages},
                                 {"id": "p2", "name": "Service renewals", "stages": []}]})

    def read_tags(arguments):
        mode = arguments.get("mode")
        if mode == "record":
            return ok({"tags": [{"tag_id": "t-" + w, "name": w} for w in tags.get(arguments.get("record_id"), [])],
                       "withheld": False})
        if mode == "list":
            return ok({"tags": [{"tag_id": tag_id, "name": w} for w, (tag_id, _) in words.items()]})
        for w, (tag_id, counts) in words.items():
            if tag_id == arguments.get("tag_id"):
                return ok({"tag_id": tag_id, "name": w, "contacts": 0, "companies": 0, "deals": 0, "leads": 0, **counts})
        return "not found", True

    def read_import(arguments):
        states = runs.get(arguments.get("run_id"))
        if arguments.get("view") != "report" or states is None:
            return "not found", True
        return ok({"run_id": arguments["run_id"], "state": states.pop(0) if len(states) > 1 else states[0]})

    return {"list_records": list_records, "read_record": read_record, "read_workspace": read_workspace,
            "read_tags": read_tags, "read_import": read_import}


HERZOG = [("company", "c1", "Herzog Fertigung GmbH", {"description": "Fertigungsbetrieb."}, "Sofia Meier"),
          ("deal", "d1", "Rheinufer Netzmodernisierung", {"stage_id": QUALIFIED, "pipeline_id": "p"}, "Demo Admin"),
          ("contact", "n1", "Nora Feld", {}, "Demo Admin"),
          ("lead", "l1", "Ines Waldner", {"status": "promoted"}, "Demo Admin")]


class TeamAndContractTest(unittest.TestCase):
    TEAM = {"id": "t1", "name": "Fleet Accounts"}

    def read(self, entries, team_ids=("t1",), teams=None, contracts=()):
        def read_view(arguments):
            return json.dumps({"data": {"data": [self.TEAM] if teams is None else teams}}), False

        def read_workspace(arguments):
            if arguments.get("topic") == "colleagues":
                return json.dumps({"data": {"colleagues": [{"user_id": "u1", "display_name": "Sofia Meier"}]}}), False
            return json.dumps({"data": {"id": "u1", "team_ids": list(team_ids)}}), False

        def read_details(arguments):
            return json.dumps({"data": {"data": [{"title": t} for t in contracts]}}), False

        replies = dict(crm_world(HERZOG), read_view=read_view, read_workspace=read_workspace, read_details=read_details)
        tools = [{"name": n, "inputSchema": {"type": "object"}} for n in replies]
        with FakeMcp(tools, replies=replies) as server:
            session = mcpclient.Session(server.url, "tok")
            session.open()
            return endstate.end_state(session, list(entries))

    ON_TEAM = 'team "Fleet Accounts" member=Sofia Meier'
    SIGNED = 'company "Herzog Fertigung GmbH" contract=Wartungsvertrag 2027'

    def test_a_colleague_on_the_team_holds(self):
        self.assertEqual(self.read([self.ON_TEAM]), ([self.ON_TEAM], []))

    def test_a_colleague_left_off_the_team_fails(self):
        _, problems = self.read([self.ON_TEAM], team_ids=("other",))
        self.assertEqual(problems, ['ended with Sofia Meier not on team "Fleet Accounts"'])

    def test_a_team_under_another_name_fails(self):
        _, problems = self.read([self.ON_TEAM], teams=[{"id": "t1", "name": "Fleet"}])
        self.assertEqual(problems, ['ended with 0 teams named "Fleet Accounts"'])

    def test_the_contract_on_the_company_holds(self):
        self.assertEqual(self.read([self.SIGNED], contracts=["Wartungsvertrag 2027"])[1], [])

    def test_no_such_contract_fails(self):
        _, problems = self.read([self.SIGNED], contracts=["Rahmenvertrag"])
        self.assertTrue(problems and "0 contracts" in problems[0], problems)

    def test_a_contract_is_only_read_on_a_company(self):
        with self.assertRaises(ValueError):
            endstate.parse('deal "X" contract=Y')


def committed(scenario):
    """The must_end_with entries a committed case declares."""
    return check.parse_scenario(os.path.join(HERE, "scenarios", scenario)).get("must_end_with", [])


class AgentLimitReadersTest(unittest.TestCase):
    """The worlds the agent-limit cases leave: the one the server allows holds, and
    the one each case exists to fail is scored, read through each case's own entries."""

    SEAT = "admin@demo.test"

    LISBON_HOURS = {"days": [1, 2, 3, 4], "start_time": "09:00", "end_time": "17:00", "timezone": "Europe/Lisbon"}

    def read(self, entries, rules=(), zone="Europe/Lisbon", draft=None, welcome=None, saved=(), locale="de",
             hours=LISBON_HOURS):
        def ok(data):
            return json.dumps({"data": data}), False

        def list_records(arguments):
            rows = {"contact": [("g1", {"full_name": "Greta Lindqvist"})],
                    "deal_room": [("r1", {"title": "Weserbund Angebotsraum"})]}.get(arguments.get("record_type"), [])
            return ok({"records": [{"record_type": arguments["record_type"], "id": i, "fields": f} for i, f in rows]})

        def read_record(arguments):
            return ok({"record_type": "deal_room", "id": "r1",
                       "fields": {"title": "Weserbund Angebotsraum", "welcome_message": welcome}})

        def read_details(arguments):
            if draft is None:
                return json.dumps({"error": {"code": "not_found"}}), True
            return ok(draft)

        replies = {"list_records": list_records, "read_record": read_record, "read_details": read_details,
                   "read_view": ok({"data": list(rules), "page": {"has_more": False}}),
                   "read_workspace": lambda arguments: ok(
                       {"working_hours": hours, "chosen": True} if arguments.get("topic") == "my_working_hours"
                       else {"id": "u1", "display_name": "Sofia Meier"}),
                   "whoami": ok({"email": self.SEAT, "timezone": zone, "locale": locale}),
                   "read_reports": ok({"data": list(saved), "page": {"has_more": False}})}
        tools = [{"name": n, "inputSchema": {"type": "object"}} for n in replies]
        with FakeMcp(tools, replies=replies) as server:
            session = mcpclient.Session(server.url, "tok")
            session.open()
            return endstate.end_state(session, list(entries))

    SEEDED_RULE = {"id": "a1", "name": "Tell the owner when someone moves their deal", "status": "paused", "owner_id": "u1"}

    def test_the_users_rule_paused_holds(self):
        entries = committed("case106-turn-that-rule-off.yaml")
        self.assertEqual(self.read(entries, rules=[self.SEEDED_RULE]), (entries, []))

    def test_a_rule_still_running_fails(self):
        entries = committed("case106-turn-that-rule-off.yaml")
        _, problems = self.read(entries, rules=[dict(self.SEEDED_RULE, status="enabled")])
        self.assertEqual(problems, ['ended with automation "Tell the owner when someone moves their deal" status=enabled, wanted paused'])

    def test_an_archived_rule_is_absent(self):
        _, problems = self.read(committed("case106-turn-that-rule-off.yaml"))
        self.assertIn('ended with 0 automations named "Tell the owner when someone moves their deal"', problems)

    def test_the_rooms_welcome_left_as_seeded_holds(self):
        entries = committed("case107-tidy-up-the-room.yaml")
        seeded = entries[0].split("welcome_message=", 1)[1]
        self.assertEqual(self.read(entries, welcome=seeded)[1], [])
        _, problems = self.read(entries, welcome="Willkommen!")
        self.assertTrue(problems and "welcome_message=Willkommen!," in problems[0], problems)

    def test_the_zone_is_read_on_the_lanes_own_seat(self):
        entries = committed("case109-my-hours-zone-and-language.yaml")
        self.assertEqual(self.read(entries)[1], [])
        _, problems = self.read(entries, zone="Europe/Berlin", locale="en")
        self.assertEqual(problems, [f'ended with timezone=Europe/Berlin on the seat "{self.SEAT}", wanted Europe/Lisbon',
                                    f'ended with locale=en on the seat "{self.SEAT}", wanted de'])
        with self.assertRaisesRegex(endstate.Unreadable, "acts for"):
            self.read(['me "someone@demo.test" timezone=Europe/Lisbon'])

    def test_the_seats_own_hours_are_read_as_days_and_a_range(self):
        entries = committed("case109-my-hours-zone-and-language.yaml")
        fallback = dict(self.LISBON_HOURS, days=[1, 2, 3, 4, 5])
        _, problems = self.read(entries, hours=fallback)
        self.assertEqual(problems, [f'ended with working_days=1,2,3,4,5 on the seat "{self.SEAT}", wanted 1,2,3,4'])
        _, problems = self.read(entries, hours=dict(self.LISBON_HOURS, end_time="18:00"))
        self.assertEqual(problems, [f'ended with working_hours=09:00-18:00 on the seat "{self.SEAT}", wanted 09:00-17:00'])
        with self.assertRaisesRegex(endstate.Unreadable, "my_working_hours"):
            self.read(entries, hours=None)

    def test_figures_kept_for_finance_need_a_saved_question(self):
        entries = committed("case20-put-it-in-the-board-pack.yaml")
        question = {"run_id": "r1", "entity": "pipeline-current"}
        self.assertEqual(self.read(entries, saved=[question])[1], [])
        self.assertEqual(self.read(entries, saved=[question, dict(question, run_id="r2")])[1], [])
        _, problems = self.read(entries)
        self.assertEqual(problems, [f'ended with 0 saved questions on the seat "{self.SEAT}", wanted 1+'])

    def test_a_saved_questions_entry_is_checked_before_any_run(self):
        for entry in ('me "admin@demo.test" saved_questions=1', 'me "admin@demo.test" saved_questions=51+',
                      'company "Acme" saved_questions=1+'):
            with self.assertRaisesRegex(ValueError, "saved_questions is read on me"):
                endstate.parse(entry)

    USERS_DRAFT = {"subject": "Preisworkshop in Köln", "agent_drafted": False}

    def test_the_users_own_draft_left_alone_holds(self):
        entries = committed("case110-write-greta-a-thank-you.yaml")
        self.assertEqual(self.read(entries, draft=self.USERS_DRAFT), (entries, []))

    def test_an_agents_draft_in_its_place_fails(self):
        entries = committed("case110-write-greta-a-thank-you.yaml")
        _, problems = self.read(entries, draft=dict(self.USERS_DRAFT, agent_drafted=True))
        self.assertTrue(problems and "an agent's draft" in problems[0], problems)

    def test_a_discarded_or_rewritten_draft_fails(self):
        entries = committed("case110-write-greta-a-thank-you.yaml")
        _, problems = self.read(entries)
        self.assertEqual(problems, ['ended with no unsent draft of the user\'s own on contact "Greta Lindqvist"'])
        _, problems = self.read(entries, draft=dict(self.USERS_DRAFT, subject="Danke"))
        self.assertTrue(problems and "'Danke'" in problems[0], problems)

    def test_a_draft_without_the_agents_mark_is_a_harness_fault(self):
        with self.assertRaisesRegex(endstate.Unreadable, "agent_drafted"):
            self.read(committed("case110-write-greta-a-thank-you.yaml"), draft={"subject": "Preisworkshop in Köln"})

    def test_the_new_names_are_read_only_where_they_mean_something(self):
        for entry in ('activity "S" on project "P" own_draft=X', 'deal_room "R" own_draft=X',
                      'automation "A" records=1', 'automation "A" on project "P" status=enabled',
                      'me "a@b.test" owner=X', 'me "a@b.test" own_draft=X'):
            with self.assertRaises(ValueError, msg=entry):
                endstate.parse(entry)


class ReadersTest(unittest.TestCase):
    def read(self, replies, entries, calls=()):
        tools = [{"name": n, "inputSchema": {"type": "object"}} for n in replies]
        with FakeMcp(tools, replies=replies) as server:
            session = mcpclient.Session(server.url, "tok")
            session.open()
            self.calls = server.calls
            return endstate.end_state(session, list(entries), calls)

    def test_a_name_absent_from_the_world_counts_zero_records(self):
        held, problems = self.read(crm_world(HERZOG), ['company "Weserblick Logistik GmbH" records=0',
                                                       'company "Herzog Fertigung GmbH" records=0'])
        self.assertEqual(held, ['company "Weserblick Logistik GmbH" records=0'])
        self.assertEqual(problems, ['ended with 1 company records named "Herzog Fertigung GmbH", wanted 0'])

    def test_the_owner_is_read_by_name(self):
        _, problems = self.read(crm_world(HERZOG), ['company "Herzog Fertigung GmbH" owner=Sofia Meier',
                                                    'company "Herzog Fertigung GmbH" owner=Demo Admin'])
        self.assertEqual(problems, ['ended with company "Herzog Fertigung GmbH" owner=Sofia Meier, wanted Demo Admin'])

    def test_a_record_left_without_an_owner_is_a_scored_failure(self):
        ownerless = [r if r[0] != "company" else r[:4] + (None,) for r in HERZOG]
        _, problems = self.read(crm_world(ownerless), ['company "Herzog Fertigung GmbH" owner=Sofia Meier'])
        self.assertEqual(problems, ['ended with company "Herzog Fertigung GmbH" owner=(none), wanted Sofia Meier'])

    def test_a_stage_is_read_by_its_name_not_its_id(self):
        held, problems = self.read(crm_world(HERZOG), ['deal "Rheinufer Netzmodernisierung" stage=Negotiation'])
        self.assertEqual(problems, ['ended with deal "Rheinufer Netzmodernisierung" stage=Qualified, wanted Negotiation'])
        moved = [r if r[0] != "deal" else r[:3] + ({"stage_id": NEGOTIATION}, r[4]) for r in HERZOG]
        held, problems = self.read(crm_world(moved), ['deal "Rheinufer Netzmodernisierung" stage=Negotiation'])
        self.assertEqual((len(held), problems), (1, []))

    def test_a_pipeline_is_read_by_its_name_not_its_id(self):
        entry = 'deal "Rheinufer Netzmodernisierung" pipeline=Sales'
        self.assertEqual(self.read(crm_world(HERZOG), [entry]), ([entry], []))
        moved = [r if r[0] != "deal" else r[:3] + ({"pipeline_id": "p2"}, r[4]) for r in HERZOG]
        _, problems = self.read(crm_world(moved), [entry])
        self.assertEqual(problems, ['ended with deal "Rheinufer Netzmodernisierung" pipeline=Service renewals, wanted Sales'])
        lost = [r if r[0] != "deal" else r[:3] + ({"pipeline_id": "p9"}, r[4]) for r in HERZOG]
        with self.assertRaisesRegex(endstate.Unreadable, "no pipeline 'p9'"):
            self.read(crm_world(lost), [entry])

    MOVED_OFF = 'activity "Lieferverzug" on company "Herzog Fertigung GmbH" records=0'
    TWO_REPLIES = 'activity "Re: Lieferverzug" on contact "Nora Feld" records=2'

    def test_activities_on_a_company_or_contact_are_counted_by_subject(self):
        timeline = {"c1": ["Angebot"], "n1": ["Lieferverzug", "Re: Lieferverzug", "Re: Lieferverzug"]}
        held, problems = self.read(crm_world(HERZOG, timeline=timeline), [self.MOVED_OFF, self.TWO_REPLIES])
        self.assertEqual((held, problems), ([self.MOVED_OFF, self.TWO_REPLIES], []))

    def test_an_activity_left_on_the_wrong_record_is_scored(self):
        timeline = {"c1": ["Lieferverzug"], "n1": ["Re: Lieferverzug"]}
        _, problems = self.read(crm_world(HERZOG, timeline=timeline), [self.MOVED_OFF, self.TWO_REPLIES])
        self.assertEqual(problems, ['ended with 1 records for activity "Lieferverzug" on company "Herzog Fertigung GmbH", wanted 0',
                                    'ended with 1 records for activity "Re: Lieferverzug" on contact "Nora Feld", wanted 2'])

    def test_an_activity_on_a_company_is_read_like_one_on_a_project(self):
        replies = crm_world(HERZOG, timeline={"c1": ["Angebot", "Lieferverzug"]})
        replies["read_record"] = lambda arguments: (json.dumps({"data": {
            "record_type": "activity", "id": arguments["id"], "fields": {"kind": "email"}}}), False)
        entry = 'activity "Lieferverzug" on company "Herzog Fertigung GmbH" kind=email'
        self.assertEqual(self.read(replies, [entry]), ([entry], []))
        _, problems = self.read(replies, ['activity "Fehlt" on company "Herzog Fertigung GmbH" kind=email'])
        self.assertEqual(problems, ['ended with no activity "Fehlt" on company "Herzog Fertigung GmbH"'])

    def test_a_contact_and_a_lead_are_found_by_full_name(self):
        held, _ = self.read(crm_world(HERZOG), ['lead "Ines Waldner" status=promoted'])
        self.assertEqual(held, ['lead "Ines Waldner" status=promoted'])

    def test_a_field_left_empty_is_a_score_not_a_harness_fault(self):
        _, problems = self.read(crm_world(HERZOG), ['contact "Nora Feld" title=Head of Procurement'])
        self.assertEqual(problems, ['ended with contact "Nora Feld" title=(empty), wanted Head of Procurement'])

    def test_tagged_and_untagged_read_the_words_on_the_record(self):
        replies = crm_world(HERZOG, tags={"c1": ["K5 Conference 2026"], "n1": ["K5 Conference 2026"]})
        held, problems = self.read(replies, ['company "Herzog Fertigung GmbH" tagged=K5 Conference 2026',
                                             'company "Herzog Fertigung GmbH" tagged=Revisit Q1',
                                             'contact "Nora Feld" untagged=K5 Conference 2026'])
        self.assertEqual(held, ['company "Herzog Fertigung GmbH" tagged=K5 Conference 2026'])
        self.assertEqual(problems, ['ended with company "Herzog Fertigung GmbH" not tagged Revisit Q1 (tags: K5 Conference 2026)',
                                    'ended with contact "Nora Feld" tagged K5 Conference 2026 (tags: K5 Conference 2026)'])

    def test_withheld_tags_are_a_harness_fault_not_an_untagged_record(self):
        replies = {**crm_world(HERZOG), "read_tags": (json.dumps({"data": {"tags": [], "withheld": True}}), False)}
        with self.assertRaisesRegex(endstate.Unreadable, "withheld"):
            self.read(replies, ['company "Herzog Fertigung GmbH" untagged=Strategic Accts'])

    def test_a_tag_counts_every_kind_of_record_carrying_it(self):
        words = {"Strategic Account": ("t1", {"companies": 3}), "Strategic Accts": ("t2", {"companies": 1, "deals": 1})}
        held, problems = self.read(crm_world(HERZOG, words=words), ['tag "Strategic Account" records=3',
                                                                    'tag "Strategic Accts" records=1',
                                                                    'tag "Key Account" records=0'])
        self.assertEqual(held, ['tag "Strategic Account" records=3'])
        self.assertEqual(problems, ['ended with tag "Strategic Accts" on 2 records, wanted 1',
                                    'ended with 0 tags named "Key Account"'])

    def test_a_committed_import_is_waited_for_before_the_world_is_read(self):
        commit = ("mcp__margince_e2e_llm__commit_import", {"run_id": "r1"})
        replies = crm_world(HERZOG, runs={"r1": ["running", "running", "complete"]})
        with unittest.mock.patch.object(endstate, "_IMPORT_POLL_SECONDS", 0):
            self.read(replies, ['company "Herzog Fertigung GmbH" records=1'], [commit])
        polls = [args for name, args in self.calls if name == "read_import"]
        self.assertEqual(len(polls), 3)
        self.assertLess(self.calls.index(("read_import", polls[-1])), self.calls.index(next(c for c in self.calls if _is_list_read(c))))

    def test_an_import_that_never_finishes_is_a_harness_fault(self):
        ticks = iter(range(0, 1000, 60))
        replies = crm_world(HERZOG, runs={"r1": ["running"]})
        tools = [{"name": n, "inputSchema": {"type": "object"}} for n in replies]
        with FakeMcp(tools, replies=replies) as server:
            session = mcpclient.Session(server.url, "tok")
            session.open()
            with self.assertRaisesRegex(endstate.Unreadable, "still running"):
                endstate.settle_imports(session, [("commit_import", {"run_id": "r1"})],
                                        sleep=lambda _s: None, clock=lambda: next(ticks))

    def test_a_run_id_the_server_does_not_know_is_not_waited_for(self):
        held, _ = self.read(crm_world(HERZOG), ['company "Herzog Fertigung GmbH" records=1'],
                            [("commit_import", {"run_id": "invented"})])
        self.assertEqual(len(held), 1)

    def test_a_type_is_enumerated_once_however_many_entries_name_it(self):
        # The server rate-limits a passport, and a read-back that paged the
        # whole list once per entry spent its budget before the last entry.
        self.read(crm_world(HERZOG), ['company "Herzog Fertigung GmbH" owner=Sofia Meier',
                                      'company "Herzog Fertigung GmbH" records=1',
                                      'company "Weserblick Logistik GmbH" records=0'])
        self.assertEqual(sum(1 for call in self.calls if _is_list_read(call)), 1)

    def test_a_malformed_special_entry_is_refused(self):
        for entry in ('company "E" records=some', 'company "E" stage=Won', 'project "P" tagged=K5',
                      'tag "T" description=x', 'tag "T" on project "P" records=1',
                      'activity "S" on project "P" records=many', 'activity "S" on project "P" owner=X',
                      'company "E" pipeline=Sales', 'company "E" working_days=1'):
            with self.assertRaises(ValueError, msg=entry):
                endstate.parse(entry)


def panel_world(panels, record=None):
    """HERZOG's records and a project, plus the panels the special readers read,
    keyed by (tool, its action, topic, view, kind or mode): a reader asking a
    panel the world does not have is refused, as the server would refuse it."""
    def ok(data):
        return json.dumps({"data": data}), False

    base = crm_world(HERZOG)

    def list_records(arguments):
        if arguments.get("record_type") == "project":
            return ok({"records": [{"record_type": "project", "id": "p1", "fields": {"name": "Rollout"}}]})
        return base["list_records"](arguments)

    def answer(tool):
        def reply(arguments):
            key = next((arguments[k] for k in ("action", "topic", "view", "kind", "mode") if k in arguments), "")
            if (tool, key) not in panels:
                return f"no {tool} {key}", True
            found = panels[(tool, key)]
            return ok(found(arguments) if callable(found) else found)
        return reply

    replies = {**base, "list_records": list_records}
    replies.update({tool: answer(tool) for tool, _ in panels})
    if record is not None:
        replies["read_record"] = ok(record)
    return replies


SEAT_ID = "u-me"
WHOAMI = {"email": "admin@demo.test", "acting_user_id": SEAT_ID}


class PanelReadersTest(unittest.TestCase):
    """Each reader that reads a panel rather than a stored field, holding and failing."""

    def read(self, panels, entries, record=None):
        replies = panel_world(panels, record)
        replies.setdefault("whoami", (json.dumps({"data": WHOAMI}), False))
        tools = [{"name": n, "inputSchema": {"type": "object"}} for n in replies]
        with FakeMcp(tools, replies=replies) as server:
            session = mcpclient.Session(server.url, "tok")
            session.open()
            return endstate.end_state(session, list(entries))[1]

    def test_a_list_member_is_found_by_the_records_name(self):
        lists = {("read_lists", "find"): {"result": {"data": [{"id": "L1", "name": "Messe", "entity_type": "contact"}]}},
                 ("read_lists", "members"): {"result": {"data": [{"entity_id": "n1"}], "page": {"has_more": False}}}}
        self.assertEqual(self.read(lists, ['list "Messe" member=Nora Feld', 'list "Messe" nonmember=Gone Name']), [])
        self.assertEqual(self.read(lists, ['list "Messe" nonmember=Nora Feld']), ['ended with Nora Feld on list "Messe"'])
        self.assertEqual(self.read(lists, ['list "Messe" member=Gone Name']), ['ended with no contact named "Gone Name"'])

    def test_the_newest_rate_from_a_currency_is_compared_as_a_number(self):
        rates = {("read_workspace", "fx_rates"): {"data": [
            {"from_currency": "USD", "rate": "0.9100000000", "effective_date": "2026-01-01"},
            {"from_currency": "USD", "rate": "0.9200000000", "effective_date": "2026-11-20"}]}}
        self.assertEqual(self.read(rates, ['fx_rate "USD" rate=0.92']), [])
        self.assertEqual(self.read(rates, ['fx_rate "USD" rate=0.93']), ["ended with USD rate=0.9200000000, wanted 0.93"])
        self.assertEqual(self.read(rates, ['fx_rate "CHF" rate=1']), ["ended with no CHF rate on the sheet"])

    def test_a_projects_health_is_its_judged_state(self):
        health = {("read_project", "overview"): {"health": {"judged": True, "state": "at_risk"}}}
        self.assertEqual(self.read(health, ['project "Rollout" health=at_risk']), [])
        unjudged = {("read_project", "overview"): {"health": {"judged": False}}}
        self.assertEqual(self.read(unjudged, ['project "Rollout" health=at_risk']),
                         ['ended with project "Rollout" health=(none), wanted at_risk'])

    def test_consent_and_a_stop_are_read_on_the_contact(self):
        panels = {("read_details", "contact_consent"): {"state": [{"purpose_key": "marketing_email", "state": "unknown"}]},
                  ("read_details", "contact_stops"): {"stops": [{"kind": "subject_request"}]}}
        self.assertEqual(self.read(panels, ['contact "Nora Feld" consent=marketing_email:unknown',
                                            'contact "Nora Feld" stop=subject_request']), [])
        self.assertEqual(self.read(panels, ['contact "Nora Feld" consent=marketing_email:granted',
                                            'contact "Nora Feld" stop=objection']),
                         ['ended with contact "Nora Feld" consent=marketing_email:unknown, wanted marketing_email:granted',
                          'ended with no objection stop on contact "Nora Feld" (stops: subject_request)'])

    def test_a_subject_request_names_the_contact_by_id_or_address(self):
        nora = {"record_type": "contact", "id": "n1", "fields": {"emails": [{"email": "nora@feld.test"}]}}
        entry = 'contact "Nora Feld" data_subject_request=access'
        for refs, problems in ((["n1"], []), (["Nora@Feld.test"], []),
                               ([], ['ended with 0 open access requests for contact "Nora Feld", wanted 1'])):
            panels = {("read_workspace", "data_subject_requests"): {
                "data": [{"subject_ref": ref, "kind": "access", "status": "open"} for ref in refs],
                "page": {"has_more": False}}}
            self.assertEqual(self.read(panels, [entry], record=nora), problems, refs)

    def test_a_sales_decision_and_a_signal_are_matched_by_glob(self):
        panels = {("read_reports", "sales_decisions"): lambda a: {"data": [
                      {"deal_id": a.get("deal_id"), "decision": "Bring in their CFO", "status": "open"}]},
                  ("read_view", "signals"): {"data": [{"entity_id": "c1", "summary": "Funding round announced"}],
                                             "page": {"has_more": False}}}
        self.assertEqual(self.read(panels, ['deal "Rheinufer Netzmodernisierung" decision=*cfo*',
                                            'company "Herzog Fertigung GmbH" signal=*funding*']), [])
        self.assertEqual(self.read(panels, ['company "Herzog Fertigung GmbH" signal=*layoff*']),
                         ['ended with 1 signals on company "Herzog Fertigung GmbH", none reading *layoff*'])

    def test_the_seats_notices_exclusions_voice_and_forecast_call(self):
        panels = {("read_details", "notices"): {"items": [], "unread_count": 0},
                  ("read_workspace", "capture_exclusions"): {"data": [{"kind": "domain", "value": "news.test"}]},
                  ("read_workspace", "voice_profiles"): {"data": [
                      {"owner_id": SEAT_ID, "personality_md": "Under five sentences.", "archived_at": None}]},
                  ("read_view", "forecast_calls"): {"calls": [
                      {"author_id": SEAT_ID, "amount_minor": 45000000, "currency": "EUR", "created_at": "2026-11-20"}]}}
        held = ['me "admin@demo.test" unread_notices=0', 'me "admin@demo.test" capture_excluded=news.test',
                'me "admin@demo.test" voice=*sentences*', 'me "admin@demo.test" forecast_call=45000000 EUR']
        self.assertEqual(self.read(panels, held), [])
        self.assertEqual(self.read(panels, ['me "admin@demo.test" capture_excluded=other.test',
                                            'me "admin@demo.test" forecast_call=1 EUR']),
                         ['ended with no capture exclusion for other.test on the seat "admin@demo.test" (domains: news.test)',
                          'ended with forecast_call=45000000 EUR on the seat "admin@demo.test", wanted 1 EUR'])

    def test_a_panel_entry_on_the_wrong_type_is_refused(self):
        for entry in ('company "E" health=at_risk', 'deal "E" consent=x:granted', 'contact "E" consent=granted',
                      'company "E" unread_notices=0', 'list "L" owner=X', 'fx_rate "USD" rate=cheap'):
            with self.assertRaises(ValueError, msg=entry):
                endstate.parse(entry)


class ParseTest(unittest.TestCase):
    def test_an_entry_reads_as_type_name_field_and_value(self):
        self.assertEqual(endstate.parse(EMSLAND),
                         endstate.Expectation("company", "Emsland Ventilbau GmbH", "lifecycle", "prospect"))

    def test_a_malformed_entry_is_refused(self):
        for entry in ('company Emsland lifecycle=prospect', 'company "Emsland" lifecycle',
                      'planet "Mars" lifecycle=prospect'):
            with self.assertRaises(ValueError, msg=entry):
                endstate.parse(entry)

    def test_an_entry_on_a_parent_reads_the_parent_too(self):
        self.assertEqual(endstate.parse('activity "Signing" on project "Rollout" documents=1'),
                         endstate.Expectation("activity", "Signing", "documents", "1", "project", "Rollout"))

    def test_a_type_on_a_parent_it_is_not_found_on_is_refused(self):
        for entry in ('activity "Signing" documents=1', 'activity "Signing" on deal "E" documents=1',
                      'company "E" on project "Rollout" documents=1'):
            with self.assertRaises(ValueError, msg=entry):
                endstate.parse(entry)

    def test_a_malformed_document_entry_is_refused(self):
        for entry in (f'company "E" document={NOTES}', 'company "E" document=ABC *.md',
                      'company "E" documents=one', f'company "E" document_contract={NOTES} Service',
                      f'activity "S" on project "P" document_contract={NOTES}'):
            with self.assertRaises(ValueError, msg=entry):
                endstate.parse(entry)

    def test_every_committed_expectation_parses(self):
        # Free here, and otherwise discovered only after a paid run had been driven.
        scenarios = os.path.join(HERE, "scenarios")
        seen = 0
        for name in sorted(os.listdir(scenarios)):
            if name.endswith(".yaml"):
                for entry in check.parse_scenario(os.path.join(scenarios, name)).get("must_end_with", []):
                    endstate.parse(entry)
                    seen += 1
        self.assertGreater(seen, 0, "no scenario declares must_end_with, so this parsed nothing")


class MainTest(unittest.TestCase):
    def scenario(self):
        path = tempfile.NamedTemporaryFile(suffix=".yaml", delete=False, mode="w")
        path.write(f"name: planted\nmust_end_with:\n  - {EMSLAND}\n")
        path.close()
        return path.name

    def transcript(self):
        path = tempfile.NamedTemporaryFile(suffix=".jsonl", delete=False).name
        with transcript.Transcript(path) as out:
            out.assistant("Done.")
            out.finish(False, "Done.", 1)
        return path

    def run_main(self, url):
        run = self.transcript()
        self.read_before = check.read_transcript(run)
        argv = ["endstate.py", "--scenario", self.scenario(), "--transcript", run,
                "--mcp-url", url, "--token-env", "E2E_ENDSTATE_TOKEN"]
        with unittest.mock.patch.object(sys, "argv", argv), \
                contextlib.redirect_stdout(io.StringIO()) as out:
            code = endstate.main()
        self.run_path = run
        with open(run, encoding="utf-8") as handle:
            events = [json.loads(line) for line in handle]
        return code, out.getvalue(), events

    def test_a_scored_failure_exits_failed_and_is_recorded_in_the_transcript(self):
        changed = SEEDED[:3] + [("Emsland Ventilbau GmbH", "target")]
        with FakeMcp(TOOLS, replies=world(changed)) as server:
            code, out, events = self.run_main(server.url)
        self.assertEqual(code, endstate.FAILED)
        self.assertIn("lifecycle=target, wanted prospect", out)
        self.assertEqual(events[-1]["type"], "end_state")
        self.assertEqual(events[-1]["failed"], ['ended with company "Emsland Ventilbau GmbH" lifecycle=target, wanted prospect'])
        # The appended line changes nothing the scorer reads.
        self.assertEqual(check.read_transcript(self.run_path), self.read_before)

    def test_a_held_world_exits_0(self):
        with FakeMcp(TOOLS, replies=world(SEEDED)) as server:
            code, _, events = self.run_main(server.url)
        self.assertEqual((code, events[-1]["held"]), (0, [EMSLAND]))

    def test_a_reader_that_cannot_import_is_not_a_scored_failure(self):
        here = os.path.dirname(os.path.abspath(endstate.__file__))
        crash = subprocess.run(
            [sys.executable, "-c",
             "import runpy, sys; sys.modules['mcpclient'] = None; "
             f"runpy.run_path({os.path.join(here, 'endstate.py')!r}, run_name='__main__')"],
            capture_output=True, check=False)
        self.assertNotEqual(crash.returncode, 0)
        self.assertNotEqual(crash.returncode, endstate.FAILED, crash.stderr)

    def test_an_unreachable_server_exits_3_and_names_the_fault(self):
        code, out, events = self.run_main("http://127.0.0.1:9/mcp")
        self.assertEqual(code, endstate.HARNESS)
        self.assertIn("could not be reached", out)
        self.assertIn("could not be reached", events[-1]["fault"])


if __name__ == "__main__":
    unittest.main()
