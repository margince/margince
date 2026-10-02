import os
import sys
import unittest

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

import mcpclient  # noqa: E402
from tests.fakes import FakeMcp  # noqa: E402

TOOLS = [
    {"name": f"t{i}", "description": "d", "inputSchema": {"type": "object"},
     "outputSchema": {"type": "object"}}
    for i in range(5)
]


class McpClientTest(unittest.TestCase):
    def test_open_returns_instructions_and_every_page_of_tools(self):
        with FakeMcp(TOOLS, instructions="call list_approvals before done", page_size=2) as server:
            instructions, tools = mcpclient.Session(server.url, "tok").open()
        self.assertEqual(instructions, "call list_approvals before done")
        self.assertEqual([tool["name"] for tool in tools], [f"t{i}" for i in range(5)])
        self.assertNotIn("outputSchema", tools[0])
        self.assertTrue(all(auth == "Bearer tok" for auth in server.auth))

    def test_sse_picks_the_matching_id(self):
        with FakeMcp(TOOLS[:1], sse=True) as server:
            session = mcpclient.Session(server.url, "tok")
            session.open()
            self.assertEqual(session.call("t0", {}), ("ok", False))

    def test_tool_error_is_a_result_not_a_fault(self):
        with FakeMcp(TOOLS[:1], replies={"t0": ("permission denied", True)}) as server:
            session = mcpclient.Session(server.url, "tok")
            session.open()
            self.assertEqual(session.call("t0", {"a": 1}), ("permission denied", True))
        self.assertEqual(server.calls, [("t0", {"a": 1})])

    def test_http_refusal_is_a_fault(self):
        with FakeMcp(TOOLS, status=401) as server:
            with self.assertRaisesRegex(mcpclient.McpFault, "401"):
                mcpclient.Session(server.url, "tok").open()

    def test_unreachable_server_is_a_fault(self):
        with self.assertRaisesRegex(mcpclient.McpFault, "could not be reached"):
            mcpclient.Session("http://127.0.0.1:9/mcp", "tok", timeout=2).open()


if __name__ == "__main__":
    unittest.main()
