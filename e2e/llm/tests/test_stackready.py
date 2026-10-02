import json
import os
import sys
import unittest

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
        self.assertEqual(self.problem(reply(3)), "")

    def test_lexical_fallback_is_named(self):
        self.assertIn("lexical", self.problem(reply(0, ["semantic_ranking_degraded_to_lexical"])))

    def test_no_hits_is_not_ready(self):
        self.assertIn("no hits", self.problem(reply(0)))

    def test_a_refused_search_is_named(self):
        self.assertIn("refused", self.problem(("not allowed", True)))


if __name__ == "__main__":
    unittest.main()
