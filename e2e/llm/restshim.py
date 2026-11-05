"""The REST door's whole surface for a CLI candidate: two tools over stdio.

A candidate on the REST door holds no MCP tools of the product. It gets the
installed skill to read and one HTTP tool scoped to the stack's `/v1`, which is
what an assistant with the Margince skill has. The skill's files are read
through `skill_file` rather than a shell, so the lane turns off every other
capability of the CLI and the surface stays exactly these two tools.

Stdlib only, like the rest of the lane: it runs on a fresh checkout.
"""

import http.client
import json
import os
import sys
import urllib.error
import urllib.parse
import urllib.request

PROTOCOL = "2025-06-18"
MAX_BYTES = 60000
MAX_LINES = 400
MAX_MATCHES = 30
METHODS = ("GET", "POST", "PUT", "PATCH", "DELETE")

TOOLS = [
    {
        "name": "skill_file",
        "description": (
            "Read a file of the Margince skill: INDEX.md lists every operation, openapi.yaml holds "
            "each one's parameters and answers, SKILL.md says how to use them. Without a path it "
            "lists the files. A long file is read in line ranges, or searched for a word."
        ),
        "inputSchema": {
            "type": "object",
            "properties": {
                "path": {"type": "string", "description": "A file of the skill, such as INDEX.md."},
                "start_line": {"type": "integer", "description": "First line to read, from 1."},
                "end_line": {"type": "integer", "description": "Last line to read."},
                "find": {"type": "string", "description": "Instead of a range: the numbered lines holding this text."},
            },
        },
    },
    {
        "name": "http",
        "description": (
            "Call the Margince API with your passport. The path is relative to /v1, such as "
            "/contacts?limit=5, and the passport is added for you."
        ),
        "inputSchema": {
            "type": "object",
            "properties": {
                "method": {"type": "string", "enum": list(METHODS)},
                "path": {"type": "string", "description": "Relative to /v1, query string included."},
                "body": {"type": "object", "description": "The JSON body of a POST, PUT or PATCH."},
            },
            "required": ["method", "path"],
        },
    },
]


def read_skill_file(root, path, start, end, find=None):
    """The lines asked of one file of the skill, or why it cannot be read."""
    if not path:
        return "\n".join(sorted(name for name in os.listdir(root) if os.path.isfile(os.path.join(root, name)))), False
    full = os.path.realpath(os.path.join(root, path))
    if os.path.dirname(full) != os.path.realpath(root) or not os.path.isfile(full):
        return f"{path} is not a file of the skill", True
    with open(full, encoding="utf-8", errors="replace") as handle:
        lines = handle.read().splitlines()
    if find:
        needle = str(find).lower()
        hits = [f"{number}: {line[:200]}" for number, line in enumerate(lines, 1) if needle in line.lower()]
        more = f"\n[{len(hits) - MAX_MATCHES} more: narrow the search]" if len(hits) > MAX_MATCHES else ""
        return f"{path}: {len(hits)} line(s) hold {find!r}\n" + "\n".join(hits[:MAX_MATCHES]) + more, False
    first = max(_whole(start, 1), 1)
    last = min(_whole(end, first + MAX_LINES - 1), first + MAX_LINES - 1, len(lines))
    text = "\n".join(lines[first - 1:last])
    if len(text) > MAX_BYTES:
        text = text[:MAX_BYTES] + "\n[cut: ask for fewer lines]"
    return f"{path} lines {first}-{last} of {len(lines)}\n{text}", False


class _NoRedirect(urllib.request.HTTPRedirectHandler):
    """Answers a redirect as the redirect it is, so the passport goes only where the lane sent it."""

    def redirect_request(self, *_args, **_kwargs):
        return None


_OPENER = urllib.request.build_opener(_NoRedirect)


def _whole(value, default):
    """A line number as a whole number, or the default for anything else."""
    try:
        return int(value)
    except (TypeError, ValueError):
        return default


def call_api(base, token, method, path, body):
    """One request to the stack's /v1, answered as the status line and the body."""
    if method not in METHODS:
        return f"the method is one of {', '.join(METHODS)}", True
    if not path.startswith("/"):
        return "the path starts with a slash and is relative to /v1", True
    if path.startswith("/v1/") or path == "/v1":
        path = path[3:]
    where, _, query = path.partition("?")
    if any(part in ("..", ".", "%2e", "%2E") or part.lower().startswith("%2e") for part in where.split("/")):
        return "the path stays under /v1", True
    # A space or an umlaut a model writes raw in a query is encoded here, as a
    # browser would, so the request is sent rather than refused by the client.
    safe = "/:@!$&'()*+,;=%"
    path = urllib.parse.quote(where, safe=safe) + ("?" + urllib.parse.quote(query, safe=safe + "?") if query else "")
    data = json.dumps(body).encode() if body is not None else None
    request = urllib.request.Request(base + path, data, method=method, headers={
        "Authorization": f"Bearer {token}", "Content-Type": "application/json", "Accept": "application/json",
    })
    try:
        with _OPENER.open(request, timeout=60) as reply:
            status, raw = reply.status, reply.read(MAX_BYTES + 1)
    except urllib.error.HTTPError as err:
        status, raw = err.code, err.read(MAX_BYTES + 1)
    except (urllib.error.URLError, OSError) as err:
        return f"the stack could not be reached: {err}", True
    text = raw.decode("utf-8", "replace")
    if len(text) > MAX_BYTES:
        text = text[:MAX_BYTES] + "\n[cut]"
    return f"HTTP {status}\n{text}", status >= 300


def run_tool(name, arguments, root, base, token):
    """One tool call as text and whether it failed. Nothing a model sends ends the server."""
    if not isinstance(arguments, dict):
        return "the arguments are an object", True
    try:
        if name == "skill_file":
            return read_skill_file(root, arguments.get("path"), arguments.get("start_line"),
                                   arguments.get("end_line"), arguments.get("find"))
        if name == "http":
            return call_api(base, token, str(arguments.get("method", "")).upper(),
                            str(arguments.get("path", "")), arguments.get("body"))
    except (ValueError, OSError, TypeError, http.client.HTTPException) as err:
        return f"the call failed: {err}", True
    return f"no tool {name}", True


def handle(message, root, base, token):
    """The reply to one JSON-RPC message, or None for a notification."""
    method, ident, params = message.get("method"), message.get("id"), message.get("params") or {}
    if ident is None:
        return None
    if method == "initialize":
        result = {
            "protocolVersion": PROTOCOL, "capabilities": {"tools": {}},
            "serverInfo": {"name": "margince-rest-door", "version": "1"},
        }
    elif method == "tools/list":
        result = {"tools": TOOLS}
    elif method == "tools/call":
        arguments = params.get("arguments") or {}
        text, failed = run_tool(params.get("name"), arguments, root, base, token)
        result = {"content": [{"type": "text", "text": text}], "isError": failed}
    elif method == "ping":
        result = {}
    else:
        return {"jsonrpc": "2.0", "id": ident, "error": {"code": -32601, "message": f"no method {method}"}}
    return {"jsonrpc": "2.0", "id": ident, "result": result}


def main():
    root, base = os.environ["MARGINCE_REST_SKILL_DIR"], os.environ["MARGINCE_REST_BASE"].rstrip("/")
    token = os.environ["MARGINCE_REST_TOKEN"]
    for line in sys.stdin:
        try:
            message = json.loads(line)
        except ValueError:
            continue
        reply = handle(message, root, base, token)
        if reply is not None:
            sys.stdout.write(json.dumps(reply) + "\n")
            sys.stdout.flush()


if __name__ == "__main__":
    main()
