#!/usr/bin/env python3
"""Whether the lane's stack serves the search a scenario's answer depends on.

search_context falls back to word overlap when no embedding model is bound or
reachable, and says so in a note. A sweep against such a stack measures every
candidate on a degraded tool and scores their recovery from it, so the lane
asks before it snapshots the world.

    stackready.py --mcp-url URL --token-env NAME [--wait SECONDS]

Exit 0 when semantic search answers; 1 only when search is confirmed lexical,
the one state E2E_LLM_ALLOW_LEXICAL may wave through; 3 for anything else — a
server that could not be asked, a refused probe, or nothing indexed by the
deadline — which the lane stops on, because none of them is a lexical search.
"""

import argparse
import json
import os
import sys
import time

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

import mcpclient  # noqa: E402

# Any seeded world answers this; its words are chosen to share little with the
# records, so only the meaning lane can rank them.
_PROBE = "clients upset when we keep changing who looks after their account"
_DEGRADED = "semantic_ranking_degraded_to_lexical"


def semantic_problem(session):
    """(why search_context is not answering semantically, whether that is a
    lexical fallback) — ("", False) when it answers by meaning."""
    text, is_error = session.call("search_context", {"query": _PROBE, "limit": 5})
    if is_error:
        return f"search_context refused the probe: {text[:200]}", False
    reply = json.loads(text)
    data = reply.get("data") if isinstance(reply, dict) else None
    notes = data.get("notes") or [] if isinstance(data, dict) else None
    if not isinstance(notes, list):
        return f"search_context answered in a shape the probe cannot read: {text[:200]}", False
    if any(isinstance(note, dict) and note.get("code") == _DEGRADED for note in notes):
        return "search_context is ranking by word overlap alone (lexical): no embedding model serves it", True
    if not data.get("hits"):
        return "search_context answered with no hits: nothing is indexed yet", False
    return "", False


def main():
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--mcp-url", required=True)
    parser.add_argument("--token-env", required=True)
    parser.add_argument("--wait", type=int, default=300, help="seconds to wait for indexing")
    args = parser.parse_args()
    session = mcpclient.Session(args.mcp_url, os.environ.get(args.token_env, ""))
    deadline = time.monotonic() + args.wait
    try:
        session.open()
        while True:
            problem, lexical = semantic_problem(session)
            if not problem:
                return 0
            if time.monotonic() >= deadline:
                print(problem)
                return 1 if lexical else 3
            time.sleep(10)
    except (mcpclient.McpFault, ValueError) as fault:
        print(f"the search probe could not be run: {fault}")
        return 3


if __name__ == "__main__":
    # A crash exits 1, which the lane would read as a confirmed lexical search;
    # whatever went wrong, it is a probe that could not be run.
    try:
        sys.exit(main())
    except Exception as crash:  # noqa: BLE001 — the backstop is the point
        print(f"the search probe crashed: {crash!r}")
        sys.exit(3)
