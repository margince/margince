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

    def test_a_field_the_read_does_not_carry_is_a_harness_fault(self):
        with self.assertRaisesRegex(endstate.Unreadable, "carries no field 'stage'"):
            self.read(world(SEEDED), ['company "Emsland Ventilbau GmbH" stage=won'])

    def test_a_transport_error_is_a_harness_fault(self):
        with FakeMcp(TOOLS, status=500) as server:
            session = mcpclient.Session(server.url, "tok")
            with self.assertRaises(mcpclient.McpFault):
                endstate.end_state(session, [EMSLAND])


class ParseTest(unittest.TestCase):
    def test_an_entry_reads_as_type_name_field_and_value(self):
        self.assertEqual(endstate.parse(EMSLAND),
                         endstate.Expectation("company", "Emsland Ventilbau GmbH", "lifecycle", "prospect"))

    def test_a_malformed_entry_is_refused(self):
        for entry in ('company Emsland lifecycle=prospect', 'company "Emsland" lifecycle',
                      'planet "Mars" lifecycle=prospect'):
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
