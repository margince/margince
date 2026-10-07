"""In-process stand-ins for the MCP server and a model provider.

Real HTTP on 127.0.0.1, so the bridge's own urllib code is what is under test;
only the far end is scripted.
"""

import json
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


class _Server:
    def __init__(self, handle):
        class Handler(BaseHTTPRequestHandler):
            def log_message(self, *_args):
                pass

            def do_POST(self):
                raw = self.rfile.read(int(self.headers.get("Content-Length") or 0))
                status, headers, payload = handle(self.path, dict(self.headers), json.loads(raw or b"{}"))
                self.send_response(status)
                for key, value in headers.items():
                    self.send_header(key, value)
                self.send_header("Content-Length", str(len(payload)))
                self.end_headers()
                self.wfile.write(payload)

        self._httpd = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        self.url = f"http://127.0.0.1:{self._httpd.server_port}"

    def __enter__(self):
        threading.Thread(target=self._httpd.serve_forever, args=(0.01,), daemon=True).start()
        return self

    def __exit__(self, *_exc):
        self._httpd.shutdown()
        self._httpd.server_close()


class FakeMcp(_Server):
    """A streamable-HTTP MCP server offering `tools`.

    replies maps a tool name to (text, is_error), or to a function of the call's
    arguments returning one, for a tool whose answer depends on what it was
    asked; sse answers every request as
    an event stream with a notification ahead of the response; page_size splits
    tools/list across cursors; status != 200 refuses every request.
    """

    def __init__(self, tools, instructions="", replies=None, sse=False, page_size=None, status=200):
        self.calls, self.auth = [], []

        def handle(_path, headers, message):
            self.auth.append(headers.get("Authorization"))
            if status != 200:
                return status, {}, b"{}"
            request_id = message.get("id")
            if request_id is None:
                return 202, {}, b""
            result = self._result(message, tools, instructions, replies or {}, page_size)
            doc = json.dumps({"jsonrpc": "2.0", "id": request_id, "result": result})
            if sse:
                note = json.dumps({"jsonrpc": "2.0", "method": "notifications/message", "params": {}})
                return 200, {"Content-Type": "text/event-stream"}, f"data: {note}\n\ndata: {doc}\n\n".encode()
            return 200, {"Content-Type": "application/json"}, doc.encode()

        super().__init__(handle)
        self.url += "/mcp"

    def _result(self, message, tools, instructions, replies, page_size):
        method = message.get("method")
        params = message.get("params") or {}
        if method == "initialize":
            return {"protocolVersion": "2025-06-18", "instructions": instructions,
                    "capabilities": {"tools": {}}}
        if method == "tools/list":
            start = int(params.get("cursor") or 0)
            end = start + page_size if page_size else len(tools)
            page = {"tools": tools[start:end]}
            if end < len(tools):
                page["nextCursor"] = str(end)
            return page
        if method != "tools/call":
            raise AssertionError(f"FakeMcp was sent {method!r}, which it does not serve")
        self.calls.append((params["name"], params.get("arguments")))
        reply = replies.get(params["name"], ("ok", False))
        text, is_error = reply(params.get("arguments") or {}) if callable(reply) else reply
        return {"content": [{"type": "text", "text": text}], "isError": is_error}


class FakeProvider(_Server):
    """A model endpoint answering with `script`, a list of (status, body) in order."""

    def __init__(self, script):
        self.requests = []
        queue = list(script)

        def handle(path, headers, body):
            self.requests.append((path, headers, body))
            if not queue:
                raise AssertionError(f"FakeProvider's script ran out at request {len(self.requests)}")
            status, doc = queue.pop(0)
            return status, {"Content-Type": "application/json"}, json.dumps(doc).encode()

        super().__init__(handle)
