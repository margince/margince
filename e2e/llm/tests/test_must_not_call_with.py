"""must_not_call_with: what a call may not carry, read from its arguments.

A note that retypes a file never shows in the answer, so only the call's own
arguments can say it was written. Both directions: the file's words in a note
fail the run, a note about the file that does not quote it passes.
"""

import os
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

import check  # noqa: E402
import transcript  # noqa: E402

FORBIDS = {"must_not_call_with": ["log_activity~AlSi9Cu3|calendar week 44"]}


def run(calls):
    path = tempfile.NamedTemporaryFile(suffix=".jsonl", delete=False).name
    with transcript.Transcript(path) as out:
        out.init("m", "gpt:api", "mcp-instructions", "connected", ["log_activity", "attach_document"])
        out.assistant("", [(f"c{i}", name, args) for i, (name, args) in enumerate(calls)])
        out.finish(False, "Done.", 1)
    return path


class MustNotCallWithTest(unittest.TestCase):
    def test_the_files_words_in_a_note_fail_whichever_field_carries_them(self):
        for field in ("body", "subject", "summary"):
            path = run([("log_activity", {"kind": "note", field: "Sample run, alloy AlSi9Cu3"})])
            problems = check.check(FORBIDS, path)
            self.assertEqual(len(problems), 1, field)
            self.assertIn("called log_activity carrying 'AlSi9Cu3'", problems[0])

    def test_a_note_about_the_file_that_does_not_quote_it_passes(self):
        path = run([("log_activity", {"kind": "note", "body": "Visit notes filed on the company."})])
        self.assertEqual(check.check(FORBIDS, path), [])

    def test_the_words_in_another_tools_call_are_not_this_entrys_subject(self):
        path = run([("attach_document", {"content_base64": "calendar week 44"})])
        self.assertEqual(check.check(FORBIDS, path), [])

    def test_a_malformed_entry_is_a_problem_not_a_pass(self):
        path = run([])
        problems = check.check({"must_not_call_with": ["log_activity.body=AlSi9Cu3"]}, path)
        self.assertEqual(problems, ["must_not_call_with entry 'log_activity.body=AlSi9Cu3' is not tool~regex"])


if __name__ == "__main__":
    unittest.main()
