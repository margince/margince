<!-- prose:plain -->
<!-- describes: frontend/src/features/automations/agentallowances.tsx frontend/src/features/automations/agentallowances.roles.tsx frontend/src/features/automations/agentallowances.form.tsx frontend/src/features/automations/agentusage.tsx backend/internal/platform/agentvolume/roles.go backend/internal/modules/identity/agentroleallowances.go backend/internal/modules/identity/settingsagentallowances.go -->
# How much an agent may do

Each agent passport may do only so much in each window. This page says what is
counted, how the count differs by role, and what an agent is told when it stops.
What an agent may do at all is in
[Agents, passports and what they may do](agents-and-passports.md).

## Allowances per window

Each passport gets an allowance per window: changes made, calls that go out,
and total calls. Every call counts once, a read too. A long list is still one
call. By default the window is 24 hours, and the allowances are 200
changes, 20 calls that go out and 1,000 calls in total. An administrator
changes them under **Settings › AI usage › Agent passport allowances**: press
**Edit allowances**, change the values, and press **Save**. One more count shows
how many AI tokens the agent used; it is only for you to see, and limits
nothing.

A connected app's passports share one allowance: a renewed passport goes on
counting where the last one stopped. MCP tool calls and API calls count against
the same allowance. Your own work in the browser is never counted.

## Allowances by role

The allowance can differ by role. Under **By role** on the same card, each role
shows its value for each kind and where the value comes from. **Set for this
role** is a value an administrator chose. **Installation** is the value at the
top of the card, which a role with no value of its own uses. For each kind, an
agent gets the highest value among the roles of the user who connected it.
Roles you made yourself work the same way.

To set a value, press **Edit allowances**, type it in the role's box, and press
**Save**. Empty a box to give the role's value up, so the role uses the
installation value again. A save changes only the boxes you changed. A role you
archived keeps its value, and has it again if you restore the role.

The built-in Admin role starts with more: 20,000 calls in total and 2,000
changes, shown as **Admin preset**. Calls that go out stay at the installation
value, because a large task is no reason to send more mail. These higher values
apply to a kind only while nobody has saved its installation value. If your
installation already had its own value, Admin keeps that value until you set one
for Admin. An empty Admin box takes the **Admin preset**. To keep Admin on the
installation value, press **Use the installation value** on its row.

A role chooses how much, never what: an Admin's agent still needs the same
permissions and the same approvals. A change of role or value resets nothing.
An agent keeps what it used this window, and a lower value refuses it on its
next call. Take Admin away from a user, and their agents drop to the lower value
on their next call. Archive a role, and its value no longer applies to anyone.

## When an allowance runs out

The allowances work differently when they run out. For changes, the agent is
refused, and a card goes to whoever approved the connection. Their approval
widens the window by one more allowance. Calls that go out and total calls are
fixed stops. No approval raises them, and only the end of the window clears
them. An agent that only reads can still reach the total and stop.

When an agent is stopped, it is told which limit stopped it, the limit and when
the window resets. Over the API the answer is a `429` with a `Retry-After`
header. The per-minute limits of an MCP connection are separate, short pauses,
the same for every role. If Margince cannot read the count for a moment, the
agent is refused with a `503` and asked to try again in a few seconds. That is
no limit, and the agent has used nothing more.

Only whoever approved the connection can answer a request to widen it. An
administrator cannot, and neither can the owner of the company. An agent's
limit is that colleague's own authority.

The window is fixed, not rolling, so every allowance starts over at the same
moment. A changed allowance applies within a minute, in the window that is
running. Raise one, and a stopped agent can go on. Lower one, and an agent
already past it is refused on its next call. A new window size waits for the
next reset, so no agent starts over early; the card says when it starts.

## Where to see usage

In **Settings → AI access**, each row under **Your connections** and **Passports**
shows the allowance closest to running out. Open a row to see all three, and,
for a stopped passport, which limit stopped it. Revoked passports are hidden;
the button under the list shows them.

**AI usage › Agent usage by user** shows how much each user's agents used, never
which passport or what it read. Each user is shown against their own allowance,
with their roles under their name. **What one user's agents get** shows one
user's allowance for each kind and the role it comes from. Administrators,
Management and operations users see it.
