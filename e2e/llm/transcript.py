"""The one writer of an e2e-llm transcript.

The shape is the subset of the claude CLI's stream-json that check.py reads, so
every driver is scored by the reader the committed fixtures already hold. A
second writer would be a second answer to what a transcript is, which is why
tests/test_transcript.py fails on one.
"""

import json
import os
import re

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


def append_end_state(path, held=(), failed=(), fault=""):
    """Append what endstate.py read of the world after the run.

    Appended after the terminal result, to a transcript any route wrote, because
    the world is read once the run is over. check.py reads no `end_state` event,
    so the line is for the reader of the kept records and changes no score.
    """
    event = {"type": "end_state", "held": list(held), "failed": list(failed)}
    if fault:
        event["fault"] = fault
    with open(path, "a", encoding="utf-8") as out:
        out.write(json.dumps(event) + "\n")


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
    answer, finished = "", False
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
        out.finish(False, answer, 1)
        return 0


# What an agy run may do besides call this lane's server. agy offers MCP tools
# lazily: the prompt names each tool, and the model reads its schema from a file
# agy keeps under its own home, then calls it through call_mcp_tool. Any other
# tool stops the run, so a Gemini verdict never rests on a file or web read.
_AGY_RESOURCE_HELPERS = {"list_resources", "read_resource"}
_AGY_SCHEMA_DIR = f"/mcp/{SERVER}/"
# A long tool result is not put in the model's context: agy writes it to this
# file of the conversation and the model reads it back.
_AGY_SPILL = re.compile(r"/\.gemini/antigravity-cli/brain/([^/]+)/\.system_generated/steps/\d+/[a-z0-9_]+\.txt$")


# The run's own folder holds nothing but the agent file, so a look into it
# reads no answer; a look anywhere else stops the run.
_AGY_LOOKS = {"read_file", "view_file", "list_dir", "find_by_name", "grep_search"}


def _inside(workspace, params):
    paths = [os.path.realpath(v) for v in params.values() if isinstance(v, str) and v.startswith("/")]
    root = os.path.realpath(workspace) if workspace else ""
    return bool(root and paths) and all(p == root or p.startswith(root + os.sep) for p in paths)


def _agy_tool(out, step, conversation, workspace=""):
    """Write one finished agy tool step; return why the run must stop, or ""."""
    name = step.get("tool_name")
    info = step.get("tool_info") or {}
    params = info.get("parameters") or {}
    path = str(params.get("Path") or "")
    if name in _AGY_LOOKS and _inside(workspace, params):
        return ""
    if name == "read_file" and _AGY_SCHEMA_DIR in path:
        return ""
    spill = _AGY_SPILL.search(os.path.realpath(path)) if path else None
    # Every step of this conversation is one the lane allows, since any other
    # stops the run, so a file agy spilled for it holds nothing from outside.
    if name == "read_file" and spill and conversation and spill.group(1) == conversation:
        return ""
    if name in _AGY_RESOURCE_HELPERS and params.get("ServerName") == SERVER:
        return ""
    # A call that names no server, or another, and ended in an error reached
    # nothing; a successful one would have brought data from outside.
    if name == "call_mcp_tool" and params.get("ServerName") != SERVER and step.get("state") == "ERROR":
        return ""
    if name == "call_mcp_tool" and params.get("ServerName") == SERVER:
        call_id = f"agy_{step.get('step_index')}"
        arguments = params.get("Arguments")
        out.assistant("", [(call_id, params.get("ToolName") or "", arguments if isinstance(arguments, dict) else {})])
        if step.get("state") == "ERROR":
            text, is_error = str((info.get("error") or {}).get("message") or "the call failed"), True
        else:
            text, is_error = str(info.get("output") or ""), False
        out.tool_results([(call_id, text, is_error)])
        return ""
    # The arguments say what the model went outside for, which is the whole of
    # the diagnosis when the prompt left something unclear.
    target = params.get("ServerName") or params.get("Path") or json.dumps(params, ensure_ascii=False)[:300]
    return f"agy used a tool outside the lane's MCP server: {name} {target}".strip()


def _agy_usage(usage):
    if "input_tokens" not in usage or "output_tokens" not in usage:
        return None
    # agy counts cached input apart from input_tokens, unlike codex; thinking is
    # billed as output, so it is counted as output.
    return {
        "input": usage["input_tokens"],
        "output": usage["output_tokens"] + (usage.get("thinking_tokens") or 0),
        "cache_read": usage.get("cache_read_tokens") or 0, "cache_write": 0, "cost": None,
    }


def from_agy(lines, path, model, driver, offered, workspace=""):
    """Write an `agy -p --output-format stream-json` run as a transcript: 0 when it ran, 3 when not.

    agy's init lists its whole tool registry, not what the model was offered, so
    `offered` is the lane's own tools/list over the same passport.
    """
    texts, conversation, result = {}, "", None
    with Transcript(path) as out:
        out.init(model, driver, "cli-default", "listed-by-lane", offered)
        for line in lines:
            try:
                event = json.loads(line)
            except ValueError:
                continue
            if event.get("event") == "result":
                result = event.get("result") or {}
                continue
            step = event.get("step_update") or {}
            conversation = conversation or event.get("conversation_id") or step.get("conversation_id") or ""
            kind, state = step.get("step_type"), step.get("state")
            if kind == "agent_response":
                texts[step.get("step_index")] = texts.get(step.get("step_index"), "") + (step.get("text_delta") or "")
                # A step with no text is the model thinking before a tool call;
                # the call itself is written as its own turn.
                if state == "DONE" and texts.get(step.get("step_index")):
                    out.assistant(texts.pop(step.get("step_index")))
            elif step.get("tool_name") and state in ("DONE", "ERROR"):
                stop = _agy_tool(out, step, conversation, workspace)
                if stop:
                    out.finish(True, f"HARNESS: {stop}", 0)
                    return 3
        if result is None:
            out.finish(True, "HARNESS: agy stopped before its turn completed", 0)
            return 3
        # A refused call measures agy's permission setting, not the model.
        if result.get("denied_actions"):
            out.finish(True, f"HARNESS: agy refused {result['denied_actions']}", 0)
            return 3
        if result.get("status") != "SUCCESS":
            out.finish(True, f"HARNESS: agy: {result.get('error') or result.get('status')}", 0)
            return 3
        out.add_usage(_agy_usage(result.get("usage") or {}))
        out.finish(False, result.get("response") or "", result.get("num_turns") or 1)
        return 0
