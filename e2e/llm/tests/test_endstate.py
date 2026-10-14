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


def crm_world(records, tags=None, words=None, runs=None):
    """A workspace over [(record_type, id, name, fields, owner_name)].

    tags maps a record id to the words read_tags lists on it; words maps a word
    to (tag_id, {kind: count}) for read_tags list and get; runs maps an import
    run id to the states read_import answers in turn, the last one repeating.
    """
    tags, words, runs = tags or {}, words or {}, {k: list(v) for k, v in (runs or {}).items()}
    name_field = {"company": "display_name", "deal": "name", "contact": "full_name", "lead": "full_name"}

    def ok(data):
        return json.dumps({"data": data}), False

    def list_records(arguments):
        rows = [{"record_type": t, "id": i, "fields": {name_field[t]: n}} for t, i, n, _, _ in records
                if t == arguments.get("record_type")]
        return ok({"records": rows})

    def read_record(arguments):
        for t, i, n, fields, owner in records:
            if i == arguments.get("id") and t == arguments.get("record_type"):
                record = {"record_type": t, "id": i, "fields": {name_field[t]: n, **fields}}
                if owner is not None:
                    record["owner"] = {"id": "u-" + owner, "name": owner, "is_you": False}
                return ok(record)
        return "not found", True

    def read_directory(arguments):
        if arguments.get("list") != "pipelines":
            return "unknown list", True
        stages = [{"id": QUALIFIED, "name": "Qualified"}, {"id": NEGOTIATION, "name": "Negotiation"}]
        return ok({"pipelines": [{"id": "p", "name": "Sales", "stages": stages}]})

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

    return {"list_records": list_records, "read_record": read_record, "read_directory": read_directory,
            "read_tags": read_tags, "read_import": read_import}


HERZOG = [("company", "c1", "Herzog Fertigung GmbH", {"description": "Fertigungsbetrieb."}, "Sofia Meier"),
          ("deal", "d1", "Rheinufer Netzmodernisierung", {"stage_id": QUALIFIED}, "Demo Admin"),
          ("contact", "n1", "Nora Feld", {}, "Demo Admin"),
          ("lead", "l1", "Ines Waldner", {"status": "promoted"}, "Demo Admin")]


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
        self.assertLess(self.calls.index(("read_import", polls[-1])), self.calls.index(next(c for c in self.calls if c[0] == "list_records")))

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
        self.assertEqual([name for name, _ in self.calls].count("list_records"), 1)

    def test_a_malformed_special_entry_is_refused(self):
        for entry in ('company "E" records=some', 'company "E" stage=Won', 'project "P" tagged=K5',
                      'tag "T" description=x', 'tag "T" on project "P" records=1',
                      'activity "S" on project "P" records=1', 'activity "S" on project "P" owner=X'):
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
        for entry in ('activity "Signing" documents=1', 'activity "Signing" on company "E" documents=1',
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
