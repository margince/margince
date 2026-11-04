<!-- prose:plain -->
# An agent does what its user does

**A user's agent can do what the user can do in the app, and no more. An action is open unless
security keeps it closed.** The point is that a user can hand real work to their agent, so a closed
action needs a reason.

## What this decides

- **Is this action open?** It is, unless there is a security reason. "An agent has no need for it" is
  not a reason, because the user decides what their agent is for.
- **How much can the agent reach?** What its user reaches: the same grants, row scope and seat, and
  only the scopes on its passport. It runs the REST route's own handler, so both doors give
  one answer.
- **What about a legal or lasting act?** It stays open, and the agent asks first. The
  tool description tells it to ask, and `ask_user_first` marks the action. Recording consent is one
  such act: the agent helps the user do it, and the user decides.
- **Who did it?** The agent. Its write names the agent, reads as no one's confirmation, and never
  replaces a value someone typed.
- **What about AI work?** It needs the `enrich` scope, because it costs AI budget. It runs at once,
  because it stays inside the product. A read starts no model call for a passport without `enrich`.

## What keeps an action closed

A security reason means the agent would get something its user could not safely give it:

- a secret or a way in, such as a credential, a webhook or an OAuth connect;
- more rights than the user gave, such as the admin role or a change to its own seat or limits;
- a way to remove data for good or take it all out, such as a full export;
- the rules a whole company runs on, such as retention policy.

## How to check it

- `x-human-only-reason` in `backend/api/crm.yaml` says why each closed action a user reaches is
  closed: `security`, `covered` or `pending`.
- `backend/gates/humanonlypending_test.go` holds the `pending` count, so it can only go down.
- `TestNoRouteBackedActionStillRefusesAnActingAgent` calls every route-backed action as a user and
  as their agent, and fails where only the agent is refused.
- `TestTheToolRoleMatrixIsPublished` measures what each role can do, in
  [mcp-tool-roles.md](../reference/mcp-tool-roles.md).

## What this does not ask for

- It gives an agent no way past a limit. A user's agent cannot see what the user cannot see.
- It does not make every act run without a word. Asking first is part of being open.
- It does not need a tool for each action. Related actions share one router, to save the words a
  client must read.
