<!-- prose:plain -->
# Contract-first

What the product exposes is set by `backend/api/crm.yaml` and the Go code
generated from it. No separate document comes before them. The order of
authority is in [the record is the code](../principles/the-record-is-the-code.md).
Below: how that contract becomes Go, and why drift blocks a merge.

## The contract decides

`backend/api/crm.yaml` (OpenAPI 3.1) is the API surface that decides.
Nothing is exposed that is not in it, and everything in it exists at
runtime from the first day:

1. `make gen` turns the 3.1 contract into a 3.0 copy (`tools/contract-overlay`)
   and runs `oapi-codegen` over it.
   That builds the request and response types and the `chi` `ServerInterface`.
   They live in `internal/contracts/`, generated and never edited by hand.
2. `tools/gen-stubs` derives one **501 stub** per contract operation
   (`internal/compose/stubs_gen.go`). Nothing embeds it. `Server` implements
   the whole interface itself, so an operation with no handler fails the build.
   A module handler that cannot serve yet answers a clear 501, never a silent 404.
3. `tools/gen-agentpolicy` derives the agent access table from the contract's
   `x-mcp-tool` and `x-agent-access` notes. It **fails the generate step** for any
   operation that changes data and carries neither. So an endpoint with no
   tier cannot ship.
4. `tools/gen-aitasks` builds the **AI task contract**
   (`backend/api/ai-tasks.yaml`, the task, tier and ladder table) into
   `internal/modules/ai/tasks_gen.go`. `tools/gen-configschema` reads the tier
   names from there into the routing shape in `config/margince.schema.json`.
   So the runtime's task registry and the operator's config check both derive
   from that contract.
5. `tools/gen-queryenums` lists every query parameter of a read that the contract
   closes to an enum (`internal/compose/queryenums_gen.go`). The server answers
   422 for a value outside the enum. A wrong filter never reads as "no rows",
   and no handler needs its own check.

## Drift is merge-blocking

`make drift` generates everything again and fails on any change. It runs
`git diff --exit-code` over `*_gen.go`, `internal/contracts/` and
`config/margince.schema.json`. That gate is part of `make check`, so:

- editing a generated file by hand fails the build;
- changing the contract without generating again fails the build;
- changing what a generate tool writes, even in one place, shows in review.

## Changing the surface

The order is always **contract first, then code**. Edit `crm.yaml`, generate again (`make gen`), and
write the handler in the module that owns the operation, in place of the generated stub. Then let
`make check` prove that the contract, the generated files and the code agree. The step-by-step list is a
how-to: **[how-to/add-an-endpoint.md](../how-to/add-an-endpoint.md)**.
