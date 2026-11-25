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


def evidence(calls):
    """What the server answered, cut for the judge: one block per call."""
    blocks, left = [], EVIDENCE_TOTAL
    for record in calls:
        if left <= 0:
            break
        name = record.get("tool", "")
        if record.get("action"):
            name += " " + record["action"]
        answer = (record.get("answer") or "")[: min(EVIDENCE_PER_CALL, left)]
        blocks.append(f"[{name} -> {record.get('outcome', '')}]\n{answer}")
        left -= len(answer)
    return "\n\n".join(blocks)


def cut(log_path, offset, out_path):
    """Copy what log_path gained past byte offset into out_path."""
    with open(log_path, "rb") as source, open(out_path, "wb") as out:
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
