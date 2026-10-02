"""Which model a candidate names on which route, and where its verdict is filed.

One table, candidates.json, so the lane, the bridge and the committed verdict
agree on a model's folder: OpenRouter's slug and the vendor's own id name one
model, and a pass rate filed under two folders would read as two models.
"""

import collections
import json
import os
import re

Route = collections.namedtuple(
    "Route", "candidate via model folder wire base_url key_env effort routing"
)

_OPENROUTER = "https://openrouter.ai/api/v1"


class RouteError(ValueError):
    """A candidate or route the table cannot serve; the message is for the operator."""


def table():
    path = os.path.join(os.path.dirname(os.path.abspath(__file__)), "candidates.json")
    with open(path, encoding="utf-8") as handle:
        return json.load(handle)


def resolve(candidate, via, model="", folder="", effort=""):
    rows = {name: row for name, row in table().items() if "routes" in row}
    if candidate not in rows:
        raise RouteError(
            f"E2E_LLM_CANDIDATE={candidate!r} is not one of: {', '.join(sorted(rows))}"
        )
    row = rows[candidate]
    if via not in row["routes"]:
        raise RouteError(
            f"{candidate} has no {via} route; it has: {', '.join(sorted(row['routes']))}"
        )
    route = row["routes"][via]
    chosen = model or route["model"]
    # An effort other than the table's is an experiment, filed apart like a
    # different model: its number must never overwrite the default's.
    experiment = bool(effort) and effort != row["effort"]
    if experiment and not re.fullmatch(r"reasoning (low|medium|high)", effort):
        raise RouteError(f"E2E_LLM_EFFORT={effort!r} must be 'reasoning low|medium|high'")
    default = chosen == route["model"] and not experiment
    base_folder = folder or (row["folder"] if default else "")
    # A '/' nests the folder one level deeper than the coverage page reads, and
    # an unknown model has no folder to borrow: both are refused before boot.
    if not base_folder or "/" in base_folder:
        raise RouteError(
            f"model {chosen!r} at effort {effort or row['effort']!r} is not this candidate's "
            "default, so set E2E_LLM_FOLDER to the folder its verdicts belong in, without a '/'"
        )
    # A CLI carries its own system prompt and tools, so its number is filed
    # beside the comparison rather than in it.
    filed = f"{base_folder}@{route['wire']}" if via == "cli" else base_folder
    base_url = route.get("base_url", "")
    if via == "openrouter":
        base_url = os.environ.get("OPENAI_COMPATIBLE_BASE_URL") or _OPENROUTER
    return Route(
        candidate, via, chosen, filed, route["wire"], base_url, route["key_env"],
        effort or row["effort"], route.get("routing"),
    )


def judge_route(via, model):
    """The judge's route: a canonical model id in, the id this route's wire names out.

    The id the judge's verdicts are recorded under stays the canonical one
    whatever the route, so a corpus recorded over the CLI replays over a key.
    """
    routes = table()["judge"]["judge_routes"]
    if via not in routes:
        raise RouteError(f"E2E_LLM_JUDGE_VIA={via!r} is not one of: cli, {', '.join(sorted(routes))}")
    route = routes[via]
    canonical = routes["api"]["model"]
    wire_model = route["model"] if model == canonical else model
    base_url = route.get("base_url", "")
    if via == "openrouter":
        base_url = os.environ.get("OPENAI_COMPATIBLE_BASE_URL") or _OPENROUTER
    return Route("judge", via, wire_model, "", route["wire"], base_url, route["key_env"], "n/a", None)
