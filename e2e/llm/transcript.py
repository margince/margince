"""The one writer of an e2e-llm transcript.

The shape is the subset of the claude CLI's stream-json that check.py reads, so
every driver is scored by the reader the committed fixtures already hold. A
second writer would be a second answer to what a transcript is, which is why
tests/test_transcript.py fails on one.
"""

import json

SERVER = "margince_e2e_llm"

# Field order is the order read_usage names them in check.py.
_USAGE = (
    ("input_tokens", "input"),
    ("output_tokens", "output"),
    ("cache_creation_input_tokens", "cache_write"),
    ("cache_read_input_tokens", "cache_read"),
)


def tool_name(bare):
    return f"mcp__{SERVER}__{bare}"


class Transcript:
    """A run's JSONL transcript, written event by event and flushed as it goes,
    so a run that dies mid-way still leaves what it did."""

    def __init__(self, path):
        self._path = path
        self._usage = dict.fromkeys((field for field, _ in _USAGE), 0)
        self._measured = True
        self._cost = None
        self._out = None

    def __enter__(self):
        self._out = open(self._path, "w", encoding="utf-8")
        return self

    def __exit__(self, *_exc):
        self._out.close()

    def _emit(self, event):
        self._out.write(json.dumps(event) + "\n")
        self._out.flush()

    def init(self, model, driver, system_prompt, status, tools):
        self._emit({
            "type": "system", "subtype": "init", "model": model, "driver": driver,
            "system_prompt": system_prompt,
            "mcp_servers": [{"name": SERVER, "status": status}],
            "tools": [tool_name(bare) for bare in tools],
        })

    def assistant(self, text="", calls=()):
        blocks = [{"type": "text", "text": text}] if text else []
        blocks += [
            {"type": "tool_use", "id": call_id, "name": tool_name(bare), "input": arguments}
            for call_id, bare, arguments in calls
        ]
        # Emitted even when empty: an empty reply is a turn the model took, and
        # check.unrun reads a transcript with no assistant turn as never run.
        self._emit({"type": "assistant", "message": {"content": blocks}})

    def tool_results(self, results):
        self._emit({"type": "user", "message": {"content": [
            {"type": "tool_result", "tool_use_id": call_id, "content": text, "is_error": is_error}
            for call_id, text, is_error in results
        ]}})

    def add_usage(self, usage):
        # One request without a usage block makes the whole run unmeasured:
        # read_usage treats a missing count as unknown, never as zero.
        if usage is None:
            self._measured = False
            return
        for field, key in _USAGE:
            self._usage[field] += usage[key]
        if usage.get("cost") is not None:
            self._cost = (self._cost or 0.0) + usage["cost"]

    def finish(self, is_error, text, turns):
        event = {
            "type": "result", "subtype": "error" if is_error else "success",
            "is_error": is_error, "result": text, "num_turns": turns,
            "total_cost_usd": self._cost,
        }
        if self._measured:
            event["usage"] = dict(self._usage)
        self._emit(event)


# What a codex run may do besides call this lane's server. Codex cannot offer
# MCP tools alone: whatever else it reaches for stops the run, so a GPT verdict
# never rests on a shell read of the scenario files.
_CODEX_HARMLESS_ITEMS = {"agent_message", "reasoning", "todo_list"}
_CODEX_RESOURCE_HELPERS = {"list_mcp_resources", "list_mcp_resource_templates", "read_mcp_resource"}


def _codex_refused(entry):
    error = entry.get("error")
    return isinstance(error, dict) and "requires approval" in str(error.get("message"))


def _codex_result_text(entry):
    if entry.get("error"):
        return str((entry["error"] or {}).get("message") or entry["error"]), True
    result = entry.get("result") or {}
    text = "\n".join(
        block.get("text", "") for block in result.get("content") or () if block.get("type") == "text"
    )
    return text, bool(result.get("isError"))


def _codex_item(out, entry):
    """Write one completed codex item; return why the run must stop, or ""."""
    kind = entry.get("type")
    if kind == "agent_message":
        out.assistant(entry.get("text") or "")
        return ""
    if kind in _CODEX_HARMLESS_ITEMS:
        return ""
    if kind == "error":
        # The stream's own failure signal: whatever it says, the run is not scored.
        return entry.get("message") or "codex reported an error"
    if kind == "mcp_tool_call" and entry.get("server") == SERVER and _codex_refused(entry):
        # Codex declined to make the call: the model never reached the tool,
        # so the run measures codex's approval setting, not the model.
        return f"codex refused to call {entry.get('tool')}: {(entry.get('error') or {}).get('message')}"
    if kind == "mcp_tool_call" and entry.get("server") == SERVER:
        # codex sends arguments as a JSON object; a string is read as one too.
        arguments = entry.get("arguments")
        if isinstance(arguments, str):
            try:
                arguments = json.loads(arguments)
            except ValueError:
                arguments = {}
        out.assistant("", [(entry["id"], entry["tool"], arguments if isinstance(arguments, dict) else {})])
        text, is_error = _codex_result_text(entry)
        out.tool_results([(entry["id"], text, is_error)])
        return ""
    if kind == "mcp_tool_call" and entry.get("server") == "codex" and entry.get("tool") in _CODEX_RESOURCE_HELPERS:
        return ""
    return f"codex used a tool outside the lane's MCP server: {kind} {entry.get('server') or ''} {entry.get('tool') or entry.get('command') or ''}".strip()


def from_codex(lines, path, model, driver, offered):
    """Write a `codex exec --json` run as a transcript: 0 when it ran, 3 when not.

    Codex emits no list of the tools it offered, so `offered` is the lane's own
    tools/list over the same passport, and the status says so.
    """
    answer, finished, lane_calls = "", False, 0
    with Transcript(path) as out:
        out.init(model, driver, "cli-default", "listed-by-lane", offered)
        for line in lines:
            try:
                event = json.loads(line)
            except ValueError:
                continue
            kind = event.get("type")
            if kind == "item.completed":
                entry = event.get("item") or {}
                if entry.get("type") == "agent_message" and entry.get("text"):
                    answer = entry["text"]
                if entry.get("type") == "mcp_tool_call" and entry.get("server") == SERVER:
                    lane_calls += 1
                stop = _codex_item(out, entry)
                if stop:
                    out.finish(True, f"HARNESS: {stop}", 0)
                    return 3
            elif kind == "turn.completed":
                usage = event.get("usage") or {}
                cached = usage.get("cached_input_tokens") or 0
                out.add_usage({
                    "input": usage.get("input_tokens", 0) - cached, "output": usage.get("output_tokens", 0),
                    "cache_read": cached, "cache_write": usage.get("cache_write_input_tokens") or 0,
                    "cost": None,
                } if "input_tokens" in usage and "output_tokens" in usage else None)
                finished = True
            elif kind == "turn.failed":
                message = (event.get("error") or {}).get("message") or "the turn failed"
                out.finish(True, f"HARNESS: codex: {message}", 0)
                return 3
        if not finished:
            out.finish(True, "HARNESS: codex stopped before its turn completed", 0)
            return 3
        # Codex reports nothing of what it attached, so a run with no call to
        # this server cannot be told from one whose tools never reached the
        # model — which once read as three failed runs of a working case.
        if not lane_calls:
            out.finish(True, "HARNESS: codex made no call to the lane's server; whether it was "
                             "attached cannot be told from its stream", 0)
            return 3
        out.finish(False, answer, 1)
        return 0
