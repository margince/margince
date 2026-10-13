#!/usr/bin/env python3
"""Read every scenario's must_end_with against a seeded stack, with no model.

Free, and the only way to find a reader fault before a paid run does: a fresh
world has done none of the errands, so each scenario should exit 0 (its entries
already hold) or 4 (not yet held), and never 3, which is an entry the reader
cannot read at all — a wrong tool, a field the read does not carry, a shape it
does not know.

    endstate_dry.py --mcp-url URL --token-env NAME [scenario.yaml ...]

Exit 1 when any scenario read 3, else 0.
"""

import argparse
import glob
import os
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)

import check  # noqa: E402
import endstate  # noqa: E402
import mcpclient  # noqa: E402


def main():
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--mcp-url", required=True)
    parser.add_argument("--token-env", required=True, help="the variable holding the MCP passport")
    parser.add_argument("scenarios", nargs="*")
    args = parser.parse_args()
    scenarios = args.scenarios or sorted(glob.glob(os.path.join(HERE, "scenarios", "*.yaml")))
    session = mcpclient.Session(args.mcp_url, os.environ.get(args.token_env, ""))
    session.open()
    faults = 0
    for scenario in scenarios:
        entries = check.parse_scenario(scenario).get("must_end_with")
        if not entries:
            continue
        # A run that called nothing: the reader waits on no import and reads
        # the world exactly as the seed left it.
        try:
            _, problems = endstate.end_state(session, entries, [])
        except (endstate.Unreadable, mcpclient.McpFault, ValueError) as fault:
            faults += 1
            print(f"{os.path.basename(scenario)}: exit {endstate.HARNESS}\n  {fault}")
            continue
        print(f"{os.path.basename(scenario)}: exit {endstate.FAILED if problems else 0}")
        for problem in problems:
            print(f"  {problem}")
    return 1 if faults else 0


if __name__ == "__main__":
    sys.exit(main())
