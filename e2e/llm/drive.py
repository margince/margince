#!/usr/bin/env python3
"""Drive one scenario prompt through a model against Margince's MCP server.

The neutral half of the lane: every vendor is offered the same tools with the
same system prompt (the server's own `instructions`, which is what an MCP host
hands its model) and the same turn cap, so verdicts filed side by side compare
models rather than the harnesses around them.

Exit 0 when the run happened, whatever the model did; exit 3 when it could not
be run (MCP unreachable, credential refused, vendor down after retries). The
lane reads 3 as a harness stop: such a run is never scored.
"""

import argparse
import json
import os
import sys
import time

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

import candidates  # noqa: E402
import mcpclient  # noqa: E402
import providers  # noqa: E402
import transcript  # noqa: E402

# The claude CLI lane's --max-turns.
MAX_TURNS = 20
# The claude CLI caps an MCP result at 25k tokens; ~4 characters a token keeps
# every candidate seeing no more than that baseline does.
RESULT_CAP = 100_000
_TRUNCATED = f"\n[truncated by the e2e-llm bridge at {RESULT_CAP} characters]"
HARNESS = 3


def _execute(session, call_id, tool, arguments):
    """One tool call as (id, text, is_error), the way an MCP host reports it.

    Arguments that are not JSON reach the model as a tool error rather than
    stopping the run: it is the model's mistake to recover from.
    """
    try:
        parsed = json.loads(arguments)
    except ValueError as err:
        return call_id, f"arguments were not valid JSON: {err}", True
    if not isinstance(parsed, dict):
        return call_id, "arguments must be a JSON object", True
    text, is_error = session.call(tool, parsed)
    if len(text) > RESULT_CAP:
        text = text[:RESULT_CAP] + _TRUNCATED
    return call_id, text, is_error


def _arguments_for_transcript(arguments):
    try:
        parsed = json.loads(arguments)
    except ValueError:
        return {}
    return parsed if isinstance(parsed, dict) else {}


def run(route, key, mcp_url, token, prompt, out_path, sleep=time.sleep):
    driver = f"{route.candidate}:{route.via}"
    with transcript.Transcript(out_path) as out:
        try:
            session = mcpclient.Session(mcp_url, token)
            instructions, tools = session.open()
        except mcpclient.McpFault as fault:
            out.init(route.model, driver, "mcp-instructions", "failed", [])
            out.finish(True, f"HARNESS: {fault}", 0)
            print(fault, file=sys.stderr)
            return HARNESS
        out.init(route.model, driver, "mcp-instructions", "connected", [t["name"] for t in tools])
        model = providers.adapter(route, key, sleep)
        model.start(instructions, prompt, tools)
        return _loop(model, session, out)


def _loop(model, session, out):
    results, answer = None, ""
    try:
        for turn_no in range(1, MAX_TURNS + 1):
            turn = model.step(results)
            out.add_usage(turn.usage)
            out.assistant(turn.text, [(i, t, _arguments_for_transcript(a)) for i, t, a in turn.calls])
            answer = turn.text or answer
            if not turn.calls:
                if not turn.text:
                    out.finish(True, "the model returned no answer", turn_no)
                else:
                    out.finish(False, turn.text, turn_no)
                return 0
            results = [_execute(session, *call) for call in turn.calls]
            out.tool_results(results)
        out.finish(True, f"stopped at the {MAX_TURNS}-turn cap", MAX_TURNS)
        return 0
    except (providers.ProviderFault, mcpclient.McpFault) as fault:
        out.finish(True, f"HARNESS: {fault}", 0)
        print(fault, file=sys.stderr)
        return HARNESS


def main():
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--candidate", required=True)
    parser.add_argument("--via", required=True)
    parser.add_argument("--model", default="")
    parser.add_argument("--folder", default="")
    parser.add_argument("--mcp-url", required=True)
    parser.add_argument("--token-env", required=True, help="the variable holding the MCP passport")
    parser.add_argument("--prompt-file", required=True)
    parser.add_argument("--out", required=True)
    args = parser.parse_args()
    try:
        route = candidates.resolve(args.candidate, args.via, args.model, args.folder)
    except candidates.RouteError as err:
        print(err, file=sys.stderr)
        return HARNESS
    key = os.environ.get(route.key_env, "")
    token = os.environ.get(args.token_env, "")
    if not key or not token:
        print(f"{route.key_env if not key else args.token_env} is not set", file=sys.stderr)
        return HARNESS
    with open(args.prompt_file, encoding="utf-8") as handle:
        prompt = handle.read()
    return run(route, key, args.mcp_url, token, prompt, args.out)


if __name__ == "__main__":
    sys.exit(main())
