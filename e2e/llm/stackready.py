#!/usr/bin/env python3
"""Whether the lane's stack serves the search a scenario's answer depends on.

search_context falls back to word overlap when no embedding model is bound or
reachable, and says so in a note. A sweep against such a stack measures every
candidate on a degraded tool and scores their recovery from it, so the lane
asks before it snapshots the world.

    stackready.py --mcp-url URL --token-env NAME [--wait SECONDS]

Exit 0 when semantic search answers, 1 (with the reason) when it never did.
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
_PROBE = "customers unhappy when we swap the person looking after them"
_DEGRADED = "semantic_ranking_degraded_to_lexical"


def semantic_problem(session):
    """Why search_context is not answering semantically, or "" when it is."""
    text, is_error = session.call("search_context", {"query": _PROBE, "limit": 5})
    if is_error:
        return f"search_context refused the probe: {text[:200]}"
    data = json.loads(text).get("data") or {}
    if any(note.get("code") == _DEGRADED for note in data.get("notes") or ()):
        return "search_context is ranking by word overlap alone (lexical): no embedding model serves it"
    if not data.get("hits"):
        return "search_context answered with no hits: nothing is indexed yet"
    return ""


def main():
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--mcp-url", required=True)
    parser.add_argument("--token-env", required=True)
    parser.add_argument("--wait", type=int, default=300, help="seconds to wait for indexing")
    args = parser.parse_args()
    session = mcpclient.Session(args.mcp_url, os.environ.get(args.token_env, ""))
    session.open()
    deadline = time.monotonic() + args.wait
    while True:
        problem = semantic_problem(session)
        if not problem:
            return 0
        if time.monotonic() >= deadline:
            print(problem)
            return 1
        time.sleep(10)


if __name__ == "__main__":
    sys.exit(main())
