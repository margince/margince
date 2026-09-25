# Agents, passports and what they may do

Margince is built to be worked in by AI agents as well as by humans. This page
is the governance half: what an agent is allowed to do, what waits for you, what
it is refused outright, and how a credential is minted and revoked.

For what the AI actually produces — drafts, document reads, briefs — see
[What the AI does, and what it does not](what-the-ai-does.md).

There is one rule underneath everything here:

> **An agent can do what the colleague behind it could do unaided — and nothing
> more. It is checked against that colleague on every single call.**

Everything else follows from that sentence. It is worth understanding properly,
because it is not the rule most readers assume.

## Connecting and controlling agents

### What is an agent passport?
An agent passport in Margince is the credential that lets one AI agent or script act as you. It carries your own seat, permissions and record visibility, narrowed to the **Agent permissions** you tick: **Read records**, **Draft messages**, **Change records**, **Send messages** and **Buy contact data**. An agent can never do more than you can. You create and revoke passports yourself in **Settings → Agents**.
Also called: API key, token, agent credential, access token.

### How do I give an agent access to Margince?
To give a script or AI agent access to Margince, open **Settings → Agents** and choose **New passport** on the **Agent passports** card.
1. Open the account menu, choose **Settings**, then **Agents**.
2. Choose **New passport**.
3. Fill **Agent name**.
4. Under **Agent permissions**, tick at least one permission.
5. Choose **Mint passport**.
6. Copy the **Credential** now: "Copy it now. This credential is shown only once."
A passport lasts 30 days. For an MCP client, use **Connect an agent** on the **Connected agents** card instead.
Also called: create an API key, connect ChatGPT or Claude, integrate an agent.

### How do I connect an MCP client such as Claude to Margince?
To connect an MCP-capable agent, open **Settings → Agents**, go to **Connected agents** and follow **Connect an agent**: run one of the commands shown, and the client registers itself. Margince then shows **Authorize access**: "{client} will be able to act in Margince as you, with the access checked below." Untick what it should not get, then choose **Authorize**, or **Deny access**.
If the page says "The MCP connector is off for this installation.", an administrator or operations user must enable it first.
Also called: MCP server, Claude Desktop, AI assistant integration.

### How do I revoke an agent's access?
To stop an agent, open **Settings → Agents**. For a passport, choose **Revoke** on it: "The passport’s credential is invalidated immediately. The agent loses access on its next call." For an MCP client, choose **Disconnect** under **Connected agents**, which ends the whole connection. Deactivating a user revokes all their passports at once.
Also called: kill switch, remove an agent, disconnect, delete an API key.

### How do I let Margince work overnight for me?
To let Margince prepare your Morning brief overnight, open **Settings → Connections** and turn on **Let Margince prepare the Morning brief overnight** on the **Overnight preparation** card. It acts as you and sees only what you can see; it reads and writes but can never send. If it says **Overnight authority expired**, turn it off and on again to renew it.
Also called: overnight agent, scheduled agent, background agent.

## The two tiers

Every action an agent can take carries one of two labels, declared once in the
product's contract and enforced the same way whether the agent arrives over MCP
or over plain HTTP.

**auto-execute.** An auto-execute action happens immediately, with the agent stamped on it
in the audit trail. This is the default, and it covers most of the surface:
looking records up, searching, summarising, drafting, ordinary field updates,
logging an activity, promoting a lead, archiving a record — and sending an email
or a message.

**confirm-first.** A confirm-first action does **not** happen. The agent's intention is
written down as an approval, and a human decides.

### Why sending is not automatically confirm-first
Agent sending surprises readers, so here is the product's own reasoning.

A passport carries the granting human's own seat, permissions and record
visibility. So a send an agent can make is one its holder could already make
unaided, sitting in the app. Requiring that same colleague to confirm it again
made the agent surface *weaker* than the colleague behind it, not safer — it added a
click without adding a check.

What is kept behind a confirmation is narrower and more specific: **the calls
whose destination the credential holder did not choose.**

### What actually waits for a human
**Enriching from the web, and reading a company's site.** The standing case. Here
the *model* names the address the server fetches. Persuading the model could
reach an address nobody holding the credential ever picked. That is a question
about where data goes, not about who is allowed to do what — so it waits.

**Creating or changing a custom field.** This changes the shape of your data for
everyone.

**Creating or changing a webhook subscription.** This decides where your events
are sent.

**Folding one tag into another.** A merge renames a word on every record
carrying it, and it is not undone by renaming it back.

**Closing or reopening a deal.** Advancing a deal is normally immediate, but a
move with a won or lost stage at *either* end waits. Winning is irreversible and
touches money; reopening takes revenue back out of a quarter that has already
been reported. If the stage's meaning cannot be read with certainty, it waits —
the doubt falls toward the approval, not past it.

**Filing an activity under a project.** Relinking is normally immediate, but
filing under a project marks the message as commercial correspondence, which is
write-once and cannot be undone by relinking away. See
[Capture](capture.md#filing-under-a-project-is-permanent).

**Any field a human last wrote.** See the section below — this one catches more
in practice than all the others together.

### Your installation can be stricter

An installation that wants every agent send confirmed can set a floor on the send
action, and it then stages as an approval exactly as anything else does. That is
an operator decision made in the deployment, not a switch in Settings.

If you need sends confirmed in your company, ask whoever runs your
installation whether that floor is set. Do not assume it.

The **Autonomy tiers** card in **Settings → Agents** states the same rule:
"Send email, book meetings, update a contact or deal: runs immediately if the
agent has that permission. Granting the permission is the approval." and
"Enrichment, custom fields, webhooks, tag merges: wait in Approvals."

See [Approvals](approvals.md) for what happens to a card once it is staged.

## An agent never has more rights than the colleague behind it

An agent does not have an identity of its own. It acts **on behalf of** a
colleague, using a credential they minted, and it is checked against that
colleague's permissions on every call.

Concretely:

- If you cannot see a record, the agent working for you cannot see it either.
  It does not get a "permission denied" that reveals the record exists — it
  gets the same nothing you would get.
- If you are not allowed to do something, the agent cannot do it by asking
  nicely. Approving a staged action demands exactly the permissions the action
  itself demands.
- The agent's credential also carries its own narrower limits (see
  **Passports** below). Having your permissions is the ceiling, not the floor.

## Things the AI is refused outright

These are not "staged for approval". They are refused.

**An agent may never release a proposal that is not its own business.** Two
rules, and the second is what makes the first worth having:

- **A credential does not release the proposal it made.** Otherwise an agent
  could stage a confirm-first action, approve its own card, and the
  confirmation would have confirmed nothing.
- **A credential does not release a proposal staged for somebody else.**
  Without this, two colleagues each lend a passport, A's agent stages the
  confirm-first call and B's approves it — and the tier has been satisfied by
  two agents with nobody having looked.

What is left is narrow and deliberate: an agent may answer a card staged for
the colleague it acts for, which is exactly what that colleague could have
answered themselves in the app.

**A step-up is the exception with no exception.** A request to widen a
credential's own allowance is never an agent's to answer, either way — a
passport that can lift its own window has none.

An agent *may* reject its own proposal. Rejecting discards, which cannot
escalate, and an agent that changes its mind should be able to take its own
request off your desk.

**An agent may never write consent or data-subject decisions.** Granting or
withdrawing consent, and fulfilling or rejecting a privacy request, are for
humans only.

**An agent may never ask a document set.** Asking a set, and defining or
changing one, are a human's work. A grounded answer is only as good as the
reader's ability to open the passage under each sentence and disagree with it,
and an agent acting on that answer unattended is precisely the reader who
cannot. This is why the question box exists for a NAMED set of documents and
not over everything.

**An agent may never change pipeline or stage configuration.** The stage ladder
is the ground truth that the "advance a deal" approval is judged against. An
agent that could edit the ladder could change the meaning of the approval it
was asking for.

For the consent, document-set and pipeline-configuration operations above, an
agent's credential is not even an accepted way to sign in: the refusal is at the
door. The approval rules are different in shape — the door admits the
credential, and the check is on the card in front of it.

**A new way to CHANGE something is refused by default.** If someone adds a
mutating action and forgets to give it a tier, agents cannot use it at all. The
failure mode of forgetting is "the agent can't do it", never "the agent can do
it unsupervised".

Be precise about the limit of that guarantee: it covers writes. A new *read*
endpoint that arrives without a tier is readable by an agent. Reads are bounded
by the credential's own permissions and record visibility instead, which is the
same ceiling the colleague behind it has.

## What actually protects a send

Agent sending is not held behind a confirmation by default, so it is worth
knowing what *does* stand in the way. Four things, and none of them is a click.

**Consent, default-deny, per purpose.** A send is refused unless an active,
proven consent grant exists for the *purpose* that send falls under. A grant for
a different purpose does not authorise it. "Unknown" blocks. "Withdrawn" blocks.
Where a purpose requires double opt-in, a granted-but-unconfirmed record does not
send. Every recipient is checked, including anyone copied in.

**Your seat, re-read at the moment of transmission.** Not at the moment of
staging. Someone demoted to a read-only seat between writing a message and its
going out is refused, whatever staged it. The message parks rather than retrying,
because a demotion is an answer, not a hiccup.

**The passport's scope.** Sending spends a `send` scope, which covers four
actions — sending an email, sending one to an account, sending a message, and
booking a meeting. A passport never granted that scope cannot do any of them,
regardless of tier. Scopes are exact: holding `write` does not imply holding
`send`.

**A hard ceiling on outward calls.** Each passport gets a fixed allowance of
calls that leave the building per 24 hours. Unlike the read and write
allowances — which a seat can widen by approving a card — **this one no
approval lifts.** It ends when the window ends.

## Human edits win, field by field

The human-edits rule is subtle and worth reading twice, because it is what
stops an agent from quietly undoing your work.

When an agent updates a record, the product looks at each field it is trying to
change and asks: **did a human last type this value?**

- Fields whose current value a human last wrote are split off and staged for
  approval — those fields only.
- Fields with no history at all are updated straight away — **except** on a
  record a human created, where a field already holding a value is treated as
  theirs and staged too. The product would rather ask twice than overwrite
  something nobody recorded the typing of.
- A proposed value identical to what is already there is never a conflict, so an
  agent re-asserting a human's own value does not raise a card.

So a mixed update partly applies and partly waits. The record comes back
updated, together with a note naming exactly which fields were withheld and
which approval card they went to. If *every* field in the update was one a
human had written, nothing applies and the whole thing waits.

When you approve that card, only the withheld fields are written. The agent's
original wider request is not replayed.

## Passports: how an agent is connected

A **passport** is the credential that binds one agent to one colleague. You mint
it yourself in **Settings → Agents**, and you can revoke it yourself. Revoking is
the kill switch for that one binding. The **Agent passports** card says: "An
agent acts with your permissions and never more: every request rechecks your
permissions."

A passport carries **scopes**, narrower rights than the colleague has. There are
five, and they are exact rather than nested: holding one never implies another.
The app names them **Agent permissions**:

- **Read records** (`read`): reads only. The only scope a read-only seat may
  spend at all.
- **Draft messages** (`draft`): proposes text. Note this is not read-only: one
  drafting action saves a draft on the deal's timeline.
- **Change records** (`write`): every change that stays inside your company.
- **Send messages** (`send`): the four actions that put something on the wire,
  booking a meeting included.
- **Buy contact data** (`enrich`): the one action that fetches from a third
  party.

A passport with only `read` can read your approvals but is refused the
decision. A passport needs `write` to approve most things, and `send` on top of
that where approving puts a message on the wire.

A passport lasts **30 days by default**, at least an hour and at most 90 days;
a passport minted in Settings always gets the 30-day default. The credential is
shown **once** and never again: "Copy it now. This credential is shown only
once." Only the hash is stored, so nobody, including an administrator, can
recover it for you.

Passports are for scripts and other tools. The page says: "An MCP client
connection does not use these; it is listed below." An MCP client gets its own
credential under **Connected agents**, which renews itself until you choose
**Disconnect**.

Revoking takes effect at the agent's next call. So does demoting the colleague
behind it: authority is re-derived every time, so a change binds mid-session
rather than at the next login.

### Letting an agent work overnight on your behalf

A scheduled agent runs while nobody is at a keyboard, so it cannot borrow your
authority from a session you are not in. It has to be given, in advance, by you.
In the app this is the **Overnight preparation** card in **Settings →
Connections**, with the switch **Let Margince prepare the Morning brief
overnight**. You were asked the same question once during onboarding.

Turning it on **mints your own passport** in the same act. That is the whole
point: the overnight run carries a credential that is yours, bound to you as
both the colleague acted for and the one who granted it, so everything it does
is limited to what you could have done yourself and is attributed to you. The
card says it "cannot send: the permission given here covers reading and writing
only, never sending." Turning it off revokes that credential rather than merely
unlinking it: the authority actually ends.

You may see a grant that you agreed to and that is still not working. That is
honest rather than broken: a passport expires on its own schedule. The card
then shows **Overnight authority expired**: "Turn this off and on again to renew
it. Until then, your Morning brief is not prepared." If Margince gained
capabilities since you agreed, it shows **Authority no longer covers the work**,
with the same remedy.

Nobody can do this on your behalf, and no agent can do it for itself. An agent
that could grant itself standing authority would be deciding its own rights.

### Volume allowances

Each passport gets a fixed allowance per 24-hour window: records read, changes
made, outward calls, and total calls. A fifth counter tracks model tokens spent;
it is advisory and governs nothing.

Two of these allowances behave differently when they run out. Reading and writing are
**step-ups** — the agent is refused, and a card goes to whoever
approved the connection, whose approval widens the window by one allowance.
Outward calls and total calls are **hard stops**: no approval lifts them, and
only the window ending clears them.

The window is fixed, not rolling, so every allowance in an installation resets at
the same moment. That is why a refusal says "when the window rolls" rather than
naming a number of hours.

Nobody but whoever approved the connection can answer a step-up. Not an
administrator, not the owner of the company. An agent's ceiling is that
colleague's own authority.

When an outside application asks to connect, you get an **Authorize access**
screen that says plainly "{client} will be able to act in Margince as you, with
the access checked below.", names the host the authorization is sent back to,
and lists the five permissions as checkboxes, all ticked by default, for you to
untick before choosing **Authorize**. Beside **Send messages** it says "sends
messages as you, without asking first". You can choose **Deny access**. There
is nothing to set up first: authorizing is what creates the connection.

## One honest gap: attachments

Everything an agent sends **to a model** is scrubbed for secrets first — API
keys, tokens, private keys, passwords are removed and replaced with a marker.

Read that scope carefully: it is the model-bound payload, not every outbound
message. An email an agent sends to a customer does not go through this pass.

This is hygiene, not a privacy filter. **Names, email addresses and phone
numbers pass through.** And the scrub matches text, while an attached file rides
along encoded, so a credential *inside an attached document* is not something it
can find.

Attaching a document is a decision to send its bytes as they are. Treat it that
way.

