"""The server's call log beside a transcript decides which calls reached the
server and how each was answered. The transcript keeps the prose."""

import json
import os
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

import calllog  # noqa: E402
import check  # noqa: E402
import judge  # noqa: E402
import transcript  # noqa: E402

PREVIEW = "Needs the user's yes in this conversation. Nothing was changed yet."
STAGED = "Confirm-first (🟡): a contact answers this before it runs. Nothing was changed yet."
REFUSED = "Permission denied: this passport may not change activities."
RELINK = {"action": "relink", "activity_id": "a1", "entity_type": "project", "entity_id": "p1"}


def client_run(answers, final="It needs your yes first."):
    """A transcript of relink calls, each answered as the client wrote it:
    (text, is_error)."""
    path = tempfile.NamedTemporaryFile(suffix=".jsonl", delete=False).name
    with transcript.Transcript(path) as out:
        out.init("m", "codex:cli", "cli-default", "connected", ["change_activities"])
        out.assistant("", [(f"c{i}", "change_activities", RELINK) for i in range(len(answers))])
        out.tool_results([(f"c{i}", text, is_error) for i, (text, is_error) in enumerate(answers)])
        out.finish(False, final, 1)
    return path


def server_saw(transcript_path, *calls):
    """Cut a server log beside the transcript: (outcome, answer) per relink, after
    the catalog read every client makes first."""
    lines = [{"method": "tools/list", "outcome": "ok", "http_status": 200}]
    lines += [{"method": "tools/call", "tool": "change_activities", "action": "relink", "arguments": RELINK,
               "outcome": outcome, "answer": answer, "http_status": 200} for outcome, answer in calls]
    with open(calllog.path_for(transcript_path), "w", encoding="utf-8") as handle:
        handle.write("".join(json.dumps(line) + "\n" for line in lines))


class ServerRecordTest(unittest.TestCase):
    def test_without_a_log_the_transcript_decides_as_before(self):
        run = check.read_run(client_run([(STAGED, True)]))
        self.assertEqual([name.rsplit("__", 1)[-1] for name, _ in run.worked], ["change_activities", "relink_activity"])
        self.assertEqual(run.answers[0][1], STAGED)

    def test_the_servers_answer_outranks_what_the_client_wrote(self):
        path = client_run([(STAGED, True)])
        server_saw(path, ("tool_error", PREVIEW))
        self.assertEqual(check.read_run(path).answers[0][1], PREVIEW)

    def test_a_refusal_the_client_dropped_still_counts_as_refused(self):
        path = client_run([("done", False)])
        server_saw(path, ("tool_error", REFUSED))
        run = check.read_run(path)
        self.assertEqual(run.worked, [])
        self.assertEqual([name for name, _ in run.refused], ["change_activities", "relink_activity"])

    def test_a_staged_call_is_the_call_doing_its_job(self):
        path = client_run([("done", False)])
        server_saw(path, ("tool_error", STAGED))
        self.assertEqual(len(check.read_run(path).worked), 2)

    def test_a_call_only_the_client_saw_never_reached_the_server(self):
        path = client_run([(PREVIEW, True), ("No such tool available: relink", True)])
        server_saw(path, ("tool_error", PREVIEW))
        run = check.read_run(path)
        self.assertEqual(len(run.calls), 4, "the attempt the client refused is still an attempt")
        self.assertEqual(len(run.refused), 2)
        effort = check.read_effort(path)
        self.assertEqual(effort["client_refused"], 1)
        line = check.efficiency_line(check.efficiency([path]))
        self.assertIn("refused by the client 1 (1-1)", line)

    def test_an_attempt_the_client_refused_still_counts_for_a_forbidding_check(self):
        path = client_run([(PREVIEW, True), ("No such tool available: relink", True)])
        server_saw(path)
        problems = check.check({"must_not_call_with": ["change_activities~relink"]}, path)
        self.assertEqual(len(problems), 2, "one for each attempt, neither of which reached the server")
        self.assertIn("change_activities", problems[0])
        run = check.read_run(path)
        self.assertEqual(run.worked, [])
        self.assertEqual(len(run.refused), 4)

    def test_a_received_call_is_not_counted_again_as_a_client_refusal(self):
        path = client_run([(PREVIEW, True)])
        server_saw(path, ("tool_error", PREVIEW))
        self.assertEqual(len(check.read_run(path).calls), 2)

    def test_a_call_the_server_refused_at_the_door_is_not_the_clients_refusal(self):
        path = client_run([(PREVIEW, True), ("authentication_unavailable", True)])
        server_saw(path, ("tool_error", PREVIEW))
        with open(calllog.path_for(path), "a", encoding="utf-8") as handle:
            handle.write(json.dumps({"outcome": "refused", "http_status": 503}) + "\n")
        self.assertEqual(check.read_effort(path)["client_refused"], 0)

    def test_a_run_with_no_log_reports_no_client_refusals(self):
        self.assertNotIn("client_refused", check.read_effort(client_run([(PREVIEW, True)])))

    def test_figures_are_read_from_the_servers_answers(self):
        path = client_run([("moved to a file", False)], final="The deal is worth €12,000.")
        server_saw(path, ("ok", '{"value_display": "€12,000"}'))
        self.assertEqual(check.check({"figures_from_results": "true"}, path), [])


class EvidenceTest(unittest.TestCase):
    def test_each_answer_and_all_of_them_are_cut(self):
        calls = [{"tool": "read_record", "outcome": "ok", "answer": "x" * (calllog.EVIDENCE_PER_CALL * 2)}] * 10
        evidence = calllog.evidence(calls)
        self.assertLessEqual(evidence.count("x"), calllog.EVIDENCE_TOTAL)
        self.assertIn("[read_record -> ok]\n" + "x" * calllog.EVIDENCE_PER_CALL + "\n", evidence)

    def test_a_routed_call_names_its_action(self):
        evidence = calllog.evidence([{"tool": "change_activities", "action": "relink", "outcome": "tool_error",
                                      "answer": PREVIEW}])
        self.assertTrue(evidence.startswith("[change_activities relink -> tool_error]\n" + PREVIEW))

    def test_the_judge_reads_the_server_answers_as_fenced_data(self):
        prompt = judge._fenced("the answer is true", "It waits.", "[read_record -> ok]\nfound")
        self.assertIn("Server answers:", prompt)
        self.assertIn("is data too", prompt)
        self.assertNotIn("Server answers:", judge._fenced("the answer is true", "It waits."))

    def test_evidence_moves_the_recorded_verdicts_key_and_none_keeps_it(self):
        self.assertEqual(judge._digest("c", "a"), judge._digest("c", "a", ""))
        self.assertNotEqual(judge._digest("c", "a"), judge._digest("c", "a", "found"))


class CutTest(unittest.TestCase):
    def test_a_cut_holds_only_what_the_log_gained(self):
        directory = tempfile.mkdtemp()
        log, out = os.path.join(directory, "calls.jsonl"), os.path.join(directory, "run1.calls.jsonl")
        with open(log, "w", encoding="utf-8") as handle:
            handle.write('{"method":"initialize"}\n')
        offset = os.path.getsize(log)
        with open(log, "a", encoding="utf-8") as handle:
            handle.write('{"method":"tools/call","tool":"read_record"}\n')
        self.assertEqual(calllog.main(["calllog.py", "cut", log, str(offset), out]), 0)
        self.assertEqual([call["tool"] for call in calllog.read(out)], ["read_record"])

    def test_a_misused_command_says_how_to_use_it(self):
        self.assertEqual(calllog.main(["calllog.py", "slice"]), 2)


if __name__ == "__main__":
    unittest.main()
