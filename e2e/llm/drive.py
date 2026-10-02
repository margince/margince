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
import subprocess
import sys
import tempfile
import time

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

import candidates  # noqa: E402
import check  # noqa: E402  — its path containment, shared rather than spelled twice
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
    results = None
    try:
        for turn_no in range(1, MAX_TURNS + 1):
            turn = model.step(results)
            out.add_usage(turn.usage)
            out.assistant(turn.text, [(i, t, _arguments_for_transcript(a)) for i, t, a in turn.calls])
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


# Every codex capability that is not a call to this lane's server, switched
# off; what codex cannot switch off, transcript.from_codex stops the run on.
# Code mode stays on: it is the path codex's MCP calls take, and without it the
# server attaches with none of its tools callable.
_CODEX_OFF = (
    "shell_tool", "unified_exec", "view_image", "multi_agent", "apps", "browser_use",
    "browser_use_external", "computer_use", "sleep_tool", "tool_suggest", "skill_search",
    "image_generation", "goals",
)
# codex has no turn cap, so a wall clock stands in for one.
CODEX_TIMEOUT = 900


def run_codex(route, mcp_url, token_env, prompt, out_path, timeout=CODEX_TIMEOUT):
    """Drive one prompt through `codex exec` and write it as a transcript.

    Run from an empty directory under a read-only sandbox, so nothing of the
    repository is within reach of a tool that slips through. The passport
    reaches codex by the name of its variable, never on the command line.
    """
    driver = f"{route.candidate}:{route.via}"
    try:
        _instructions, tools = mcpclient.Session(mcp_url, os.environ.get(token_env, "")).open()
    except mcpclient.McpFault as fault:
        with transcript.Transcript(out_path) as out:
            out.init(route.model, driver, "cli-default", "failed", [])
            out.finish(True, f"HARNESS: {fault}", 0)
        print(fault, file=sys.stderr)
        return HARNESS
    with tempfile.TemporaryDirectory(prefix="e2e-llm-codex.") as empty:
        argv = [
            "codex", "exec", "--json", "--strict-config", "--ignore-user-config", "--skip-git-repo-check",
            "-C", empty, "--sandbox", "read-only", "-m", route.model,
            "-c", f'mcp_servers.{transcript.SERVER}.url="{mcp_url}"',
            "-c", f'mcp_servers.{transcript.SERVER}.bearer_token_env_var="{token_env}"',
            "-c", 'web_search="disabled"',
        ]
        for feature in _CODEX_OFF:
            argv += ["-c", f"features.{feature}=false"]
        argv.append(prompt)
        try:
            done = subprocess.run(argv, stdin=subprocess.DEVNULL, capture_output=True, text=True,
                                  timeout=timeout, check=False)
            lines, stderr, exited = done.stdout.splitlines(), done.stderr, done.returncode
        except subprocess.TimeoutExpired as expired:
            # The partial stream is converted anyway: it ends without a completed
            # turn, which from_codex reports as a run that never finished.
            partial = expired.stdout or b""
            lines = (partial.decode("utf-8", "replace") if isinstance(partial, bytes) else partial).splitlines()
            stderr, exited = f"codex overran its {timeout}s wall clock and was killed", None
    code = transcript.from_codex(lines, out_path, route.model, driver, [t["name"] for t in tools])
    # A codex that exits non-zero failed as a program (config, auth, a renamed
    # feature key under --strict-config) whatever its stream says.
    if exited:
        code = HARNESS
    if code:
        print(stderr.strip()[-2000:], file=sys.stderr)
    return code


def main():
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--candidate", required=True)
    parser.add_argument("--via", required=True)
    parser.add_argument("--model", default="")
    parser.add_argument("--folder", default="")
    parser.add_argument("--effort", default="")
    parser.add_argument("--mcp-url", required=True)
    parser.add_argument("--token-env", required=True, help="the variable holding the MCP passport")
    parser.add_argument("--prompt-file", required=True)
    parser.add_argument("--out", required=True)
    args = parser.parse_args()
    try:
        route = candidates.resolve(args.candidate, args.via, args.model, args.folder, args.effort)
    except candidates.RouteError as err:
        print(err, file=sys.stderr)
        return HARNESS
    try:
        with check._open_checked(args.prompt_file) as handle:
            prompt = handle.read()
    except (ValueError, OSError) as err:
        print(err, file=sys.stderr)
        return HARNESS
    if route.wire == "codex-cli":
        return run_codex(route, args.mcp_url, args.token_env, prompt, args.out)
    key = os.environ.get(route.key_env, "")
    token = os.environ.get(args.token_env, "")
    if not key or not token:
        print(f"{route.key_env if not key else args.token_env} is not set", file=sys.stderr)
        return HARNESS
    return run(route, key, args.mcp_url, token, prompt, args.out)


if __name__ == "__main__":
    # Any crash is the bridge's, not the model's: exit 3 so the lane stops
    # instead of scoring a run that never happened.
    try:
        sys.exit(main())
    except Exception as crash:  # noqa: BLE001 — the backstop is the point
        print(f"the bridge crashed: {crash!r}", file=sys.stderr)
        sys.exit(HARNESS)
