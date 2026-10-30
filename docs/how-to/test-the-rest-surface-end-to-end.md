<!-- prose:plain -->
# Test the REST API end to end with a real assistant

Find out whether a real assistant can do a daily job through the REST API, with the skill
from `GET /v1/agent-bundle` and a passport. This is the same lane as
[test-the-mcp-surface-end-to-end.md](test-the-mcp-surface-end-to-end.md), and the same
scenarios. Read that page first: it covers the stack, the search, the credentials and the cost
of a model. The integration tests already show that one input gets one answer on both
routes. This lane asks whether a model can find and use a REST operation from the skill alone.

## Run one scenario

```bash
MARGINCE_E2E_LLM=1 E2E_LLM_CANDIDATE=claude E2E_LLM_VIA=cli E2E_LLM_DOOR=rest \
  E2E_LLM_STACK_PRESET=config/presets/gemini_cloud.yaml \
  SCENARIO=case61_what_have_i_promised make e2e-llm
```

Only a CLI can use REST mode, so `E2E_LLM_VIA=cli` is needed, with `claude` or `gpt`.

## What the assistant gets

The lane downloads the skill with the admin session, as a user does, and gives the assistant two
tools and nothing else. `e2e/llm/restshim.py` serves them.

| Tool | What it does |
|---|---|
| `skill_file` | Reads a file of the skill (`INDEX.md`, `openapi.yaml`, `SKILL.md`), a range of lines at a time, or lists the lines that hold a word. |
| `http` | Calls `/v1` with the passport. The path is relative to `/v1`. |

## How a call is graded

The scenarios name agent verbs, so each `http` call is read as the verb its operation declares
in the contract. `e2e/llm/rest_ops.json` is that table, and the Go tests derive it from the contract.
A verb that MCP lists behind a router is read as the call to that router. So `must_call` holds the same in
both modes, and so does a `must_call_with` on the argument that picks the action, such as `change_tags.action=remove`.

The path, query and body parameters of a call become its arguments. Two names change to the
MCP names: `entity_type` becomes `record_type` and `entity_id` becomes `record_id`. Every other parameter keeps
the name the REST route gives it. A `must_call_with` on one of those, such as `tag_id`, can then fail
in REST mode when the assistant did the right thing. Read the transcript first.

## The verdicts

The verdicts go to `backend/internal/compose/aicert/records/mcp_e2e_rest/<folder>/` and say
`door: rest`. [mcp-tool-coverage.md](../reference/mcp-tool-coverage.md) does not read that folder yet.

## What it costs

A run on Claude over the CLI cost about `$0.16` at API prices. Five scenarios cost `$0.23`, `$0.39`,
`$0.51`, `$0.15` and `$0.33` for two runs each. The assistant reads the skill first, so a REST run
is longer than an MCP run. A sweep of every scenario at two or three runs each would cost about `$11–17`.
That number is worked out from those five, and it is not a measured sweep. Run one scenario before a sweep.
