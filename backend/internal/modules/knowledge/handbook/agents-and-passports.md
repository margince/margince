# Agents, passports and what they may do

Margince is built to be worked in by AI agents as well as by humans. Here you
find what an agent is allowed to do, what waits for you, what it is refused
outright, and how a passport is created and revoked.

For what the AI produces (drafts, document reads, briefs), see
[What the AI does, and what it does not](what-the-ai-does.md).

There is one rule underneath everything here:

> An agent can do what the colleague behind it could do unaided, and nothing
> more. It is checked against that colleague on every call.

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

Every action an agent can take has one of two labels. The label applies however
the agent connects.

**auto-execute.** An auto-execute action happens immediately, with the agent
named on it in the audit trail. This is the default, and it covers most of what
an agent does: looking records up, searching, summarising, drafting, ordinary
field updates, logging an activity, promoting a lead, archiving a record, and
sending an email or a message.

**confirm-first.** A confirm-first action does not happen. The agent's intention
is written down as an approval, and a human decides.

### Why sending is not automatically confirm-first
A passport carries the seat, permissions and record visibility of the colleague who
created it. So a send an agent can make is one that colleague could already make
unaided, sitting in the app. Asking that same colleague to confirm it again
would add a click without adding a check.

What is kept behind a confirmation is narrower: the calls whose destination
the passport holder did not choose.

### What actually waits for a human
**Enriching from the web.** This includes reading a company's site. Here the
AI names the address Margince fetches. Someone who persuades the AI could make
it reach an address nobody holding the passport ever picked. That is a question
about where your data goes, so it waits.

**Creating or changing a custom field.** This changes the shape of your data for
everyone.

**Creating or changing a webhook subscription.** This decides where your events
are sent.

**Folding one tag into another.** A merge renames a word on every record
carrying it, and it is not undone by renaming it back.

**Closing or reopening a deal.** Advancing a deal is normally immediate, but a
move with a won or lost stage at *either* end waits. Winning cannot be undone
and touches money; reopening takes revenue back out of a quarter that has
already been reported. If Margince cannot tell what a stage means, the move
waits.

**Filing an activity under a project.** Relinking is normally immediate, but
filing under a project marks the message as commercial correspondence, which
relinking away does not remove. A member can take the filing back with **Undo
filing**; an agent cannot. See
[Capture](capture.md#filing-under-a-project-starts-a-retention-clock).

**Any field a human last wrote.** See the section below. In practice this
catches more than all the others together.

### Your installation can be stricter

An installation that wants every agent send confirmed can set a floor on
sending, and a send then waits in Approvals like anything else. Whoever runs
your installation sets this; there is no switch for it in Settings.

If you need sends confirmed in your company, ask whoever runs your
installation whether that floor is set. Do not assume it.

The **Autonomy tiers** card in **Settings → Agents** states the same rule:
"Send email, book meetings, update a contact or deal: runs immediately if the
agent has that permission. Granting the permission is the approval." and
"Enrichment, custom fields, webhooks, tag merges: wait in Approvals."

See [Approvals](approvals.md) for what happens to a card once it is staged.

## An agent never has more rights than the colleague behind it

An agent does not have an identity of its own. It acts *on behalf of* a
colleague, using a passport they created, and it is checked against that
colleague's permissions on every call.

In practice:

- If you cannot see a record, the agent working for you cannot see it either.
  It does not get a "permission denied" that reveals the record exists; it
  gets the same nothing you would get.
- If you are not allowed to do something, the agent cannot do it by asking
  nicely. Approving a staged action needs the same permissions as the action
  itself.
- The passport also carries its own narrower limits (see **Passports** below).
  Your permissions are the most an agent can ever have.

## Things the AI is refused outright

Margince refuses these; they never reach Approvals.

**Approving someone else's proposals.** An agent may approve a proposal only
in these cases:

- An agent never approves a proposal staged for somebody else. Otherwise two
  colleagues could each lend a passport, one agent could stage a confirm-first
  action and the other approve it, and nobody would have looked.
- An agent may approve its own proposal only when the change could be undone.
  That excludes closing a deal, relinking, merging tags, changing a custom
  field, sending, and anything your installation always holds for a human.
  Any of the colleague's passports may approve such a proposal, including the
  one that proposed it and a connected agent.
- Anything else an agent proposed is approved by the colleague in the app, or
  through a passport they created by hand themselves. Neither the passport that
  proposed it nor a connected agent may approve it, because nobody had to be
  present for either.

So an agent may answer a card staged for the colleague it acts for, its own
proposal included, within the rules above. That is what the colleague could
have answered in the app, and could put back afterwards. A scheduled agent
never answers a card at all, because nobody is watching it to have said yes.

**Raising its own allowance.** An agent can never answer a request to raise its
own allowance, either way. A passport that could lift its own limit would have
no limit.

An agent *may* reject its own proposal. Rejecting discards the proposal and
cannot widen anything, so an agent that changes its mind can take its own
request off your desk.

**Consent and privacy decisions.** Granting or withdrawing consent, and
fulfilling or rejecting a privacy request, are for humans only.

**Asking a document set.** Asking a set, and defining or changing one, are a
human's work. A grounded answer is only as good as the reader's ability to
open the passage under each sentence and disagree with it. An agent acting on
that answer unattended cannot do that. This is why the question box works over
a named set of documents rather than over everything.

**Changing the pipeline or its stages.** The stage ladder is what the "advance
a deal" approval is judged against. An agent that could edit the ladder could
change the meaning of the approval it was asking for.

For consent, document sets and pipeline settings, Margince does not let an
agent's passport sign in at all. Approvals work differently: the passport is
let in, and the check happens on the card itself.

**New actions that change data.** Agents cannot use a new action that changes
data until it is given a label. Forgetting the label means the agent cannot do
it, never that it can do it unsupervised.

This covers changes only. A new way to *read* data is open to an agent as soon
as it exists. Reading is still limited by the passport's own permissions and
record visibility, the same ceiling the colleague behind it has.

## What actually protects a send

Agent sending is not held behind a confirmation by default, so it helps to know
what *does* stand in the way. Four things, and none of them is a click.

**Consent, refused by default, per purpose.** A send is refused unless an
active, proven consent exists for the *purpose* that send falls under. Consent
for a different purpose does not count. "Unknown" blocks. "Withdrawn" blocks.
Where a purpose requires double opt-in, consent that was granted but not yet
confirmed does not send. Every recipient is checked, including anyone copied in.

**Your seat, checked again at sending.** It is checked when the message goes out, not only
when the message is staged. Someone moved to a read-only seat between writing a
message and its going out is refused, whatever staged it. The message stays
parked and is not retried, because the demotion still applies.

**The passport's permissions.** Sending needs **Send messages**, which covers
four actions: sending an email, sending one to an account, sending a message,
and booking a meeting. A passport without it cannot do any of them, whatever
the tier. **Change records** does not include **Send messages**. Each
permission must be ticked on its own.

**A hard ceiling on outward calls.** Each passport gets a fixed allowance of
calls that leave Margince per 24 hours. The read and write allowances can be
widened by approving a card; this one cannot. It ends when the window ends.

## Human edits win, field by field

This rule stops an agent from overwriting what you typed.

When an agent updates a record, Margince looks at each field it is trying to
change and asks: did a human last type this value?

- Fields whose current value a human last wrote are split off and staged for
  approval. Only those fields wait.
- Fields with no history at all are updated straight away. The *exception* is
  a record a human created: there, a field already holding a value is treated
  as theirs and staged too. Margince would rather ask twice than overwrite
  something nobody recorded the typing of.
- A proposed value identical to what is already there is never a conflict, so an
  agent repeating your own value does not raise a card.

So a mixed update partly applies and partly waits. The record comes back
updated, with a note naming which fields were held back and which approval card
they went to. If *every* field in the update was one a human had written,
nothing applies and the whole update waits.

When you approve that card, only the held-back fields are written. The agent's
original, wider request is not replayed.

## Passports: how an agent is connected

A **passport** is the credential that ties one agent to one colleague. You
create it yourself in **Settings → Agents**, and you can revoke it yourself.
Revoking cuts off that one agent. The **Agent passports** card says: "An agent
acts with your permissions and never more: every request rechecks your
permissions."

A passport carries permissions narrower than the colleague has. Each one stands
alone: ticking one never includes another. The app names them **Agent
permissions**:

- **Read records**: reads only. The only permission a read-only seat may use at
  all.
- **Draft messages**: proposes text. This is not read-only: one drafting action
  saves a draft on the deal's timeline.
- **Change records**: every change that stays inside your company.
- **Send messages**: the four actions that send something out of Margince,
  booking a meeting included.
- **Buy contact data**: the one action that fetches from an outside provider.

A passport with only **Read records** can read your approvals but cannot decide
them. A passport needs **Change records** to approve most things, and **Send
messages** as well where approving sends a message.

A passport lasts 30 days by default, at least an hour and at most 90 days; a
passport created in Settings always gets the 30-day default. The credential is
shown once and never again: "Copy it now. This credential is shown only once."
Margince keeps no readable copy, so nobody, including an administrator, can
recover it for you.

Passports are for scripts and other tools. The page says: "An MCP client
connection does not use these; it is listed below." An MCP client gets its own
credential under **Connected agents**, which renews itself until you choose
**Disconnect**.

Revoking takes effect at the agent's next call. So does demoting the colleague
behind it: Margince checks the colleague's rights on every call, so a change
applies mid-session rather than at the next sign-in.

### Letting an agent work overnight on your behalf

A scheduled agent runs while nobody is at a keyboard, so it cannot borrow your
rights from a session you are not in. You have to give them in advance. In the
app this is the **Overnight preparation** card in **Settings → Connections**,
with the switch **Let Margince prepare the Morning brief overnight**. You were
asked the same question once during onboarding.

Turning it on creates your own passport in the same step. The overnight run
then carries a passport that is yours, so everything it does is limited to
what you could have done yourself and is recorded under your name. The card
says it "cannot send: the permission given here covers reading and writing
only, never sending." Turning it off revokes that passport, so the authority
ends.

A passport expires after 30 days, so a grant you agreed to can stop working.
The card then shows **Overnight authority expired**: "Turn this off and on again
to renew it. Until then, your Morning brief is not prepared." If Margince gained
capabilities since you agreed, it shows **Authority no longer covers the work**,
with the same remedy.

Nobody can do this on your behalf, and no agent can do it for itself. An agent
that could grant itself standing authority would be deciding its own rights.

### Volume allowances

Each passport gets a fixed allowance per 24-hour window: records read, changes
made, outward calls, and total calls. A fifth counter tracks how much AI use
the agent spent; it is for information only and limits nothing.

The allowances behave differently when they run out. For reading and writing,
the agent is refused and a card goes to whoever approved the connection; their
approval widens the window by one more allowance. Outward calls and total calls
are hard stops: no approval lifts them, and only the window ending clears them.

The window is fixed, not rolling, so every allowance in an installation resets at
the same moment. That is why a refusal says "when the window rolls" rather than
naming a number of hours.

Only whoever approved the connection can answer a request to widen it. An
administrator cannot, and neither can the owner of the company. An agent's
ceiling is that colleague's own authority.

When an outside application asks to connect, you get an **Authorize access**
screen. It says "{client} will be able to act in Margince as you, with the
access checked below.", names the address the authorization is sent back to,
and lists the five permissions as checkboxes. All are ticked by default, and
you untick what it should not get before choosing **Authorize**. Beside
**Send messages** it says "sends messages as you, without asking first". You
can choose **Deny access**. There is nothing to set up first: authorizing
creates the connection.

## A gap: attachments

Everything an agent sends *to the AI model* is scrubbed for secrets first: API
keys, tokens, private keys and passwords are removed and replaced with a
marker.

This applies only to what goes to the model. An email an agent sends to a
customer is not scrubbed.

The scrub removes secrets; it does not protect personal data. Names, email
addresses and phone numbers pass through. The scrub also reads only text, so a
secret *inside an attached document* is not something it can find.

Attaching a document is a decision to send it as it is. Treat it that way.
