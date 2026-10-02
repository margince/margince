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
