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
import subprocess
import sys
import tempfile

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)

import check  # noqa: E402
import endstate  # noqa: E402
import transcript  # noqa: E402


def main():
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--mcp-url", required=True)
    parser.add_argument("--token-env", required=True, help="the variable holding the MCP passport")
    parser.add_argument("scenarios", nargs="*")
    args = parser.parse_args()
    scenarios = args.scenarios or sorted(glob.glob(os.path.join(HERE, "scenarios", "*.yaml")))
    faults = 0
    with tempfile.TemporaryDirectory() as work:
        for scenario in scenarios:
            if not check.parse_scenario(scenario).get("must_end_with"):
                continue
            # A run that called nothing: the reader waits on no import and
            # reads the world exactly as the seed left it.
            run = os.path.join(work, os.path.basename(scenario) + ".jsonl")
            with transcript.Transcript(run) as out:
                out.finish(False, "", 0)
            read = subprocess.run(
                [sys.executable, os.path.join(HERE, "endstate.py"), "--scenario", scenario, "--transcript", run,
                 "--mcp-url", args.mcp_url, "--token-env", args.token_env],
                capture_output=True, text=True, check=False)
            print(f"{os.path.basename(scenario)}: exit {read.returncode}")
            for line in read.stdout.splitlines():
                print(f"  {line.strip()}")
            if read.returncode not in (0, endstate.FAILED):
                faults += 1
    return 1 if faults else 0


if __name__ == "__main__":
    sys.exit(main())
