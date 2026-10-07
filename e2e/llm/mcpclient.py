"""A streamable-HTTP MCP client for the e2e-llm bridge, stdlib only.

The lane runs on a fresh checkout with neither the Go toolchain nor an SDK, so
this is the small part of an MCP host the bridge needs: initialize, list every
page of tools, call them.
"""

import json
import urllib.error
import urllib.request

PROTOCOL = "2025-06-18"


class McpFault(Exception):
    """The server could not be reached or answered outside the protocol: a
    harness fault, never something the model did."""


class Session:
    def __init__(self, url, token, timeout=60):
        self._url = url
        self._token = token
        self._timeout = timeout
        self._next_id = 0
        self._session_id = None

    def _post(self, message):
        headers = {
            "Content-Type": "application/json",
            "Accept": "application/json, text/event-stream",
            "Authorization": f"Bearer {self._token}",
            "MCP-Protocol-Version": PROTOCOL,
        }
        if self._session_id:
            headers["Mcp-Session-Id"] = self._session_id
        request = urllib.request.Request(
            self._url, json.dumps(message).encode(), headers, method="POST"
        )
        try:
            with urllib.request.urlopen(request, timeout=self._timeout) as response:
                self._session_id = response.headers.get("Mcp-Session-Id") or self._session_id
                return response.headers.get("Content-Type", ""), response.read().decode()
        except urllib.error.HTTPError as err:
            detail = err.read().decode("utf-8", "replace")[:300]
            raise McpFault(
                f"the MCP server answered HTTP {err.code} to {message.get('method')}: {detail}"
            ) from err
        except (urllib.error.URLError, TimeoutError, ConnectionError) as err:
            raise McpFault(f"the MCP server could not be reached: {err}") from err

    def _request(self, method, params=None):
        self._next_id += 1
        kind, body = self._post(
            {"jsonrpc": "2.0", "id": self._next_id, "method": method, "params": params or {}}
        )
        # An event stream may carry notifications ahead of the response, so the
        # response is the event whose id is this request's.
        try:
            if "event-stream" in kind:
                docs = [json.loads(line[5:]) for line in body.splitlines() if line.startswith("data:")]
            else:
                docs = [json.loads(body)]
        except ValueError as err:
            raise McpFault(f"{method}: the server answered with something that is not JSON-RPC: {body[:200]!r}") from err
        for doc in docs:
            if not isinstance(doc, dict) or doc.get("id") != self._next_id:
                continue
            if "error" in doc:
                raise McpFault(f"{method} failed: {doc['error'].get('message')}")
            if "result" not in doc:
                raise McpFault(f"{method}: the response carries no result and no error")
            return doc["result"]
        raise McpFault(f"{method}: no response carried id {self._next_id}")

    def open(self):
        """Initialize and list every tool: (the server's instructions, tools).

        outputSchema is dropped from each tool: the bridge offers the model the
        input contract, which is all a function-calling API accepts.
        """
        init = self._request("initialize", {
            "protocolVersion": PROTOCOL, "capabilities": {},
            "clientInfo": {"name": "e2e-llm-bridge", "version": "1"},
        })
        self._post({"jsonrpc": "2.0", "method": "notifications/initialized"})
        tools, cursor = [], None
        while True:
            page = self._request("tools/list", {"cursor": cursor} if cursor else {})
            tools += [
                {key: value for key, value in tool.items() if key != "outputSchema"}
                for tool in page.get("tools", [])
            ]
            cursor = page.get("nextCursor")
            if not cursor:
                return init.get("instructions") or "", tools

    def call(self, name, arguments):
        """One tool call: (its text content, whether the server marked it an error)."""
        result = self._request("tools/call", {"name": name, "arguments": arguments})
        text = "\n".join(
            block.get("text", "") for block in result.get("content", []) if block.get("type") == "text"
        )
        return text, bool(result.get("isError"))
