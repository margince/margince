#!/usr/bin/env python3
"""The server's own record of one run's MCP calls.

The api appends a JSON line per /mcp exchange to the file `mcp.call_log` names
(backend/internal/modules/agents/calllog.go). The lane cuts each run's lines
out beside its transcript. Where that cut is there, it decides which calls
reached the server and how each was answered. A client that drops or rewrites
a field of a result can then no longer change a verdict. The client's
transcript still holds the prose, and the attempts a client refused itself.

    calllog.py cut LOG OFFSET OUT    the lines LOG gained past byte OFFSET, into OUT
"""
import json
import os
import sys

# The cut beside a transcript: <transcript>.calls, outside every *.jsonl glob
# the lane and its tests read transcripts by.
SUFFIX = ".calls"

# The judge reads at most this much of each answer, and of all of them, so a
# long read cannot crowd out the criterion.
EVIDENCE_PER_CALL = 2000
EVIDENCE_TOTAL = 12000

# What every answer repeats and no claim rests on. It is about a third of a
# call's share, spent before the call's own data begins.
ENVELOPE = ("schema_version", "trace_id", "freshness")
STOCK_WARNING = "untrusted_content"


def path_for(transcript_path):
    return transcript_path + SUFFIX


def read(path):
    """The tool calls one cut holds, in the order the server answered them."""
    calls = []
    with open(path, encoding="utf-8") as handle:
        for line in handle:
            line = line.strip()
            if not line:
                continue
            record = json.loads(line)
            if record.get("method") == "tools/call":
                calls.append(record)
    return calls


def door_refusals(path):
    """How many requests the server refused at the door, before it read which
    method they asked for: a missing or failed credential, or pacing."""
    with open(path, encoding="utf-8") as handle:
        records = [json.loads(line) for line in handle if line.strip()]
    return sum(1 for record in records if record.get("outcome") == "refused" and not record.get("method"))


def worked(record):
    """Whether the server ran the call. A confirm-first answer is a tool error
    the caller tells apart by its words, as it does in a transcript."""
    return record.get("outcome") == "ok"


def compact(answer):
    """An answer without its envelope: the ids, the freshness and the stock
    warning every answer carries. Any other warning stays, and text that is not
    a JSON object is kept as the server sent it."""
    try:
        body = json.loads(answer)
    except ValueError:
        return answer
    if not isinstance(body, dict):
        return answer
    for key in ENVELOPE:
        body.pop(key, None)
    warnings = [w for w in body.pop("warnings", None) or []
                if not (isinstance(w, dict) and w.get("code") == STOCK_WARNING)]
    if warnings:
        body["warnings"] = warnings
    return json.dumps(body, ensure_ascii=False, separators=(",", ":"))


def evidence(calls):
    """What the server answered, cut for the judge: one block per call, in the
    order the calls were made.

    The budget is spent from the last call back. A write, and the read that
    checks it, come after the lookups, and they are what an answer that says
    it is done rests on. The calls left out are counted, so the judge does not
    read their absence as the server saying nothing.
    """
    blocks, left = [], EVIDENCE_TOTAL
    for record in reversed(calls):
        if left <= 0:
            break
        name = record.get("tool", "")
        if record.get("action"):
            name += " " + record["action"]
        answer = compact(record.get("answer") or "")[: min(EVIDENCE_PER_CALL, left)]
        blocks.append(f"[{name} -> {record.get('outcome', '')}]\n{answer}")
        left -= len(answer)
    blocks.reverse()
    if len(blocks) < len(calls):
        blocks.insert(0, f"[{len(calls) - len(blocks)} earlier calls are not shown; "
                         "nothing here says what they returned]")
    return "\n\n".join(blocks)


def cut(log_path, offset, out_path):
    """Copy what log_path gained past byte offset into out_path. Both stay inside
    the repo or the system temp directory, as every path the lane opens does."""
    import check  # check reads cuts through this module, so it imports it first

    with open(check.checked_path(log_path), "rb") as source, open(check.checked_path(out_path), "wb") as out:
        source.seek(offset)
        out.write(source.read())


def main(argv):
    if len(argv) != 5 or argv[1] != "cut":
        print(__doc__.strip().splitlines()[-1].strip(), file=sys.stderr)
        return 2
    cut(argv[2], int(argv[3]), argv[4])
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
