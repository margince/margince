import contextlib
import io
import json
import os
import sys
import unittest
import unittest.mock

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

import mcpclient  # noqa: E402
import stackready  # noqa: E402
from tests.fakes import FakeMcp  # noqa: E402

SEARCH = [{"name": "search_context", "inputSchema": {"type": "object"}}]


def reply(hits, notes=()):
    return json.dumps({"data": {"hits": [{"id": str(i)} for i in range(hits)],
                                "notes": [{"code": code} for code in notes]}}), False


class SemanticTest(unittest.TestCase):
    def problem(self, text_and_error):
        with FakeMcp(SEARCH, replies={"search_context": text_and_error}) as server:
            session = mcpclient.Session(server.url, "tok")
            session.open()
            return stackready.semantic_problem(session)

    def test_semantic_hits_are_ready(self):
        self.assertEqual(self.problem(reply(3)), ("", False))

    def test_lexical_fallback_is_named_as_lexical(self):
        why, lexical = self.problem(reply(0, ["semantic_ranking_degraded_to_lexical"]))
        self.assertEqual((lexical, "lexical" in why), (True, True))

    def test_no_hits_is_not_ready_and_not_lexical(self):
        why, lexical = self.problem(reply(0))
        self.assertEqual((lexical, "no hits" in why), (False, True))

    def test_a_refused_search_is_not_lexical(self):
        why, lexical = self.problem(("not allowed", True))
        self.assertEqual((lexical, "refused" in why), (False, True))


class MainTest(unittest.TestCase):
    def run_main(self, url):
        argv = ["stackready.py", "--mcp-url", url, "--token-env", "T", "--wait", "0"]
        with unittest.mock.patch.object(sys, "argv", argv), \
                contextlib.redirect_stdout(io.StringIO()) as out:
            return stackready.main(), out.getvalue()

    def test_an_unreachable_server_is_a_harness_fault_not_a_lexical_search(self):
        code, out = self.run_main("http://127.0.0.1:9/mcp")
        self.assertEqual(code, 3)
        self.assertIn("could not be reached", out)

    def test_a_reply_that_is_not_json_is_a_harness_fault(self):
        with FakeMcp(SEARCH, replies={"search_context": ("<html>", False)}) as server:
            code, out = self.run_main(server.url)
        self.assertEqual(code, 3)

    def test_a_stack_that_never_indexed_is_a_harness_fault_not_lexical(self):
        with FakeMcp(SEARCH, replies={"search_context": reply(0)}) as server:
            code, out = self.run_main(server.url)
        self.assertEqual((code, "no hits" in out), (3, True))

    def test_a_lexical_search_is_exit_1_with_its_reason(self):
        with FakeMcp(SEARCH, replies={"search_context": reply(0, ["semantic_ranking_degraded_to_lexical"])}) as server:
            code, out = self.run_main(server.url)
        self.assertEqual((code, "lexical" in out), (1, True))


if __name__ == "__main__":
    unittest.main()
