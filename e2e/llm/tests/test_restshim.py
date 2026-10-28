import json
import os
import sys
import tempfile
import threading
import unittest
from http.server import BaseHTTPRequestHandler, HTTPServer

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

import restshim  # noqa: E402


class _Api(BaseHTTPRequestHandler):
    seen = []

    def _answer(self):
        length = int(self.headers.get("Content-Length") or 0)
        _Api.seen.append((self.command, self.path, self.headers.get("Authorization"), self.rfile.read(length)))
        status = 404 if self.path.startswith("/v1/missing") else 200
        body = json.dumps({"path": self.path}).encode()
        self.send_response(status)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    do_GET = do_POST = do_PATCH = _answer

    def log_message(self, *_args):
        pass


class ShimTest(unittest.TestCase):
    def setUp(self):
        self.root = tempfile.TemporaryDirectory()
        with open(os.path.join(self.root.name, "INDEX.md"), "w", encoding="utf-8") as handle:
            handle.write("\n".join(f"line {n}" for n in range(1, 1001)))
        self.server = HTTPServer(("127.0.0.1", 0), _Api)
        threading.Thread(target=self.server.serve_forever, daemon=True).start()
        self.base = f"http://127.0.0.1:{self.server.server_port}/v1"
        _Api.seen.clear()

    def tearDown(self):
        self.server.shutdown()
        self.server.server_close()
        self.root.cleanup()

    def call(self, name, arguments):
        message = {"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": name, "arguments": arguments}}
        result = restshim.handle(message, self.root.name, self.base, "tok")["result"]
        return result["content"][0]["text"], result["isError"]

    def test_the_surface_is_the_skill_and_one_http_tool(self):
        listed = restshim.handle({"jsonrpc": "2.0", "id": 1, "method": "tools/list"}, self.root.name, self.base, "tok")
        self.assertEqual([tool["name"] for tool in listed["result"]["tools"]], ["skill_file", "http"])

    def test_a_notification_gets_no_reply(self):
        self.assertIsNone(restshim.handle({"jsonrpc": "2.0", "method": "notifications/initialized"}, self.root.name, self.base, "tok"))

    def test_a_skill_file_is_read_in_a_range(self):
        text, failed = self.call("skill_file", {"path": "INDEX.md", "start_line": 10, "end_line": 12})
        self.assertFalse(failed)
        self.assertEqual(text.splitlines(), ["INDEX.md lines 10-12 of 1000", "line 10", "line 11", "line 12"])

    def test_a_read_is_capped_and_says_where_it_stopped(self):
        text, _ = self.call("skill_file", {"path": "INDEX.md"})
        self.assertTrue(text.startswith(f"INDEX.md lines 1-{restshim.MAX_LINES} of 1000"))

    def test_no_path_lists_the_files(self):
        self.assertEqual(self.call("skill_file", {}), ("INDEX.md", False))

    def test_a_path_outside_the_skill_is_refused(self):
        for path in ("../secret", "/etc/hosts", "sub/../../x"):
            text, failed = self.call("skill_file", {"path": path})
            self.assertTrue(failed, path)
            self.assertIn("not a file of the skill", text)

    def test_a_call_carries_the_passport_and_the_path_under_v1(self):
        text, failed = self.call("http", {"method": "post", "path": "/contacts", "body": {"a": 1}})
        self.assertFalse(failed)
        self.assertTrue(text.startswith("HTTP 200"))
        self.assertEqual(_Api.seen, [("POST", "/v1/contacts", "Bearer tok", b'{"a": 1}')])

    def test_a_v1_prefix_is_not_doubled(self):
        self.call("http", {"method": "GET", "path": "/v1/contacts"})
        self.assertEqual(_Api.seen[0][1], "/v1/contacts")

    def test_an_error_status_is_an_error_result(self):
        text, failed = self.call("http", {"method": "GET", "path": "/missing"})
        self.assertTrue(failed)
        self.assertTrue(text.startswith("HTTP 404"))

    def test_a_raw_space_or_umlaut_in_a_query_is_encoded_and_the_server_keeps_running(self):
        for query in ("Acme Corp", "Müller"):
            text, failed = self.call("http", {"method": "GET", "path": f"/contacts?search={query}"})
            self.assertFalse(failed, query)
            self.assertTrue(text.startswith("HTTP 200"))
        self.assertEqual(_Api.seen[0][1], "/v1/contacts?search=Acme%20Corp")
        self.assertEqual(_Api.seen[1][1], "/v1/contacts?search=M%C3%BCller")

    def test_a_path_that_climbs_out_of_v1_is_refused(self):
        for path in ("/../metrics", "/contacts/../../healthz", "/%2e%2e/x"):
            text, failed = self.call("http", {"method": "GET", "path": path})
            self.assertTrue(failed, path)
        self.assertEqual(_Api.seen, [])

    def test_a_method_outside_the_list_is_refused(self):
        _text, failed = self.call("http", {"method": "TRACE", "path": "/contacts"})
        self.assertTrue(failed)
        self.assertEqual(_Api.seen, [])

    def test_a_search_names_the_lines_that_hold_a_word(self):
        text, failed = self.call("skill_file", {"path": "INDEX.md", "find": "LINE 99"})
        self.assertFalse(failed)
        self.assertTrue(text.startswith("INDEX.md: 11 line(s) hold 'LINE 99'"))
        self.assertIn("99: line 99", text)

    def test_arguments_that_are_not_what_a_tool_takes_are_a_result_not_a_crash(self):
        self.assertTrue(self.call("skill_file", {"path": "INDEX.md", "start_line": "first"})[0].startswith("INDEX.md lines 1-"))
        message = {"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": "http", "arguments": "oops"}}
        self.assertTrue(restshim.handle(message, self.root.name, self.base, "tok")["result"]["isError"])

    def test_a_path_without_a_slash_is_refused_before_any_request(self):
        _text, failed = self.call("http", {"method": "GET", "path": "contacts"})
        self.assertTrue(failed)
        self.assertEqual(_Api.seen, [])


if __name__ == "__main__":
    unittest.main()
