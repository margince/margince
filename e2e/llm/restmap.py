"""Grades a REST call as the agent verb it runs.

The scenarios name verbs (`review_commitments`, `update_record`), not routes. On
the REST door the candidate calls routes, so each call is rewritten to the verb
its operation declares as its `x-mcp-tool`, and the checker then reads the run
as it reads one made over MCP. The table is e2e/llm/rest_ops.json, which the Go
suite derives from the contract.
"""

import argparse
import json
import os
import re
import sys
from urllib.parse import parse_qsl, urlsplit

import mcpclient
import transcript

HERE = os.path.dirname(os.path.abspath(__file__))
OPERATIONS = os.path.join(HERE, "rest_ops.json")
ROUTES = os.path.join(HERE, "routes.json")
HTTP_TOOL = transcript.tool_name("http")
# The two routes that take a record by kind and id name them entity_type and
# entity_id, in the path or in the body; the MCP tools say record_type and
# record_id. Other parameters keep their names, so a criterion on one of those
# may not hold on this route.
ALIASES = {"entity_type": "record_type", "entity_id": "record_id"}


def routers(path=ROUTES):
    """Each verb a router fronts as (router, the argument naming the action, the action).

    The scenarios name the router an assistant is offered, and check.py credits a
    router call as the verb it ran. A REST call carries only the verb, so it is
    turned back into the router call that runs it.
    """
    with open(path, encoding="utf-8") as handle:
        table = json.load(handle)["routers"]
    return {
        verb: (name, router.get("field", ""), action)
        for name, router in table.items()
        for action, verb in router["actions"].items()
    }


def load(path=OPERATIONS):
    """Each operation as (method, pattern, parameter names, verb), most specific first."""
    with open(path, encoding="utf-8") as handle:
        table = json.load(handle)["operations"]
    out = []
    for route, verb in table.items():
        method, template = route.split(" ", 1)
        names = re.findall(r"\{(\w+)\}", template)
        pattern = re.compile("^" + re.sub(r"\\\{\w+\\\}", "([^/]+)", re.escape(template)) + "$")
        out.append((method, pattern, names, verb, len(re.sub(r"\{\w+\}", "", template))))
    return [entry[:4] for entry in sorted(out, key=lambda entry: -entry[4])]


def grade(operations, method, path):
    """The verb and the path and query parameters of a call, or None for a call that runs no verb."""
    parts = urlsplit(path)
    route = parts.path.rstrip("/") or "/"
    if not (route == "/v1" or route.startswith("/v1/")):
        route = "/v1" + route
    for known, pattern, names, verb in operations:
        found = pattern.match(route)
        if known == str(method).upper() and found:
            named = {ALIASES.get(name, name): value for name, value in zip(names, found.groups())}
            return verb, {**dict(parse_qsl(parts.query)), **named}
    return None


def rewrite_call(operations, fronted, call):
    """One `http` tool_use as the call that runs the same verb over MCP.

    The path parameters win over a body key of the same name, and the router
    that fronts the verb is named with the argument that picks the action.
    """
    arguments = call.get("input") or {}
    if not isinstance(arguments, dict):
        return call
    graded = grade(operations, arguments.get("method", ""), str(arguments.get("path", "")))
    if graded is None:
        return call
    verb, parameters = graded
    body = arguments.get("body") if isinstance(arguments.get("body"), dict) else {}
    merged = {ALIASES.get(key, key): value for key, value in {**body, **parameters}.items()}
    if verb in fronted:
        router, field, action = fronted[verb]
        return {**call, "name": transcript.tool_name(router), "input": {**merged, **({field: action} if field else {})}}
    return {**call, "name": transcript.tool_name(verb), "input": merged}


def rewrite(lines, operations, fronted, offered):
    """A REST run's transcript as a run over the MCP surface, offered as that surface offers it."""
    out = []
    for line in lines:
        try:
            event = json.loads(line)
        except ValueError:
            # A line check.py cannot read, such as a stream cut mid-write, is
            # left for it to skip as it does on the MCP door.
            out.append(line)
            continue
        if event.get("type") == "system" and event.get("subtype") == "init":
            event = {**event, "door": "rest", "tools": [transcript.tool_name(name) for name in offered]}
        elif event.get("type") == "assistant":
            blocks = (event.get("message") or {}).get("content") or []
            blocks = [
                rewrite_call(operations, fronted, block) if block.get("type") == "tool_use" and block.get("name") == HTTP_TOOL else block
                for block in blocks
            ]
            event = {**event, "message": {**event["message"], "content": blocks}}
        out.append(json.dumps(event))
    return out


def main(argv):
    """restmap.py TRANSCRIPT --mcp-url URL --token-env VAR: rewrite the transcript in place."""
    parser = argparse.ArgumentParser(description=main.__doc__.splitlines()[0])
    parser.add_argument("transcript")
    parser.add_argument("--mcp-url", required=True)
    parser.add_argument("--token-env", required=True)
    args = parser.parse_args(argv[1:])
    path = args.transcript
    try:
        _instructions, tools = mcpclient.Session(args.mcp_url, os.environ.get(args.token_env, "")).open()
    except mcpclient.McpFault as fault:
        print(fault, file=sys.stderr)
        return 3
    offered = [tool["name"] for tool in tools]
    with open(path, encoding="utf-8") as handle:
        lines = [line for line in handle.read().splitlines() if line.strip()]
    with open(path, "w", encoding="utf-8") as handle:
        handle.write("\n".join(rewrite(lines, load(), routers(), offered)) + "\n")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
