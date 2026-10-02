import json
import os
import sys
import unittest

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

import mcpclient  # noqa: E402
from tests.fakes import FakeMcp, _Server  # noqa: E402

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
        # initialize, initialized, three pages of tools/list
        self.assertEqual(server.auth, ["Bearer tok"] * 5)

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


class MalformedReplyTest(unittest.TestCase):
    def serve(self, status, body):
        return _Server(lambda _p, _h, _m: (status, {"Content-Type": "application/json"}, body))

    def test_a_reply_that_is_not_json_is_a_fault(self):
        with self.serve(200, b"<html>proxy</html>") as server:
            with self.assertRaisesRegex(mcpclient.McpFault, "not JSON-RPC"):
                mcpclient.Session(server.url, "tok").open()

    def test_a_response_with_neither_result_nor_error_is_a_fault(self):
        with self.serve(200, json.dumps({"jsonrpc": "2.0", "id": 1}).encode()) as server:
            with self.assertRaisesRegex(mcpclient.McpFault, "no result"):
                mcpclient.Session(server.url, "tok").open()

    def test_a_refusal_carries_the_servers_reason(self):
        with self.serve(401, b'{"detail":"passport expired"}') as server:
            with self.assertRaisesRegex(mcpclient.McpFault, "passport expired"):
                mcpclient.Session(server.url, "tok").open()


if __name__ == "__main__":
    unittest.main()
