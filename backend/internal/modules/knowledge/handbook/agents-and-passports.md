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

## The two tiers

Every action an agent can take carries one of two labels, declared once in the
product's contract and enforced the same way whether the agent arrives over MCP
or over plain HTTP.

**auto-execute.** The action happens immediately, with the agent stamped on it
in the audit trail. This is the default, and it covers most of the surface:
looking records up, searching, summarising, drafting, ordinary field updates,
logging an activity, promoting a lead, archiving a record — and sending an email
or a message.

**confirm-first.** The action does **not** happen. The agent's intention is
written down as a card in the approval inbox, and a human decides.

### Why sending is not automatically confirm-first

This surprises readers, so here is the product's own reasoning.

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

An installation that wants every send confirmed can set a floor on the send
action, and it then stages into the inbox exactly as anything else does. That is
an operator decision made in the deployment, not a switch in Settings.

If you need sends confirmed in your company, ask whoever runs your
installation whether that floor is set. Do not assume it.

> **A note on wording.** Some screens in the app still describe an older, stricter
> rule — "Write & send wait for you", "we never send anything without your
> approval". The contract the server actually enforces is the one described
> above. Where a screen and this page disagree, the behaviour above is what
> happens.

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

Since sending is not held behind a confirmation by default, it is worth knowing
what *does* stand in the way. Four things, and none of them is a click.

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

This is a subtle rule and worth reading twice, because it is what stops an
agent from quietly undoing your work.

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
it yourself in Settings, and you can revoke it yourself. Revoking is the kill
switch for that one binding.

A passport carries **scopes** — narrower rights than the colleague has. There are
five, and they are exact rather than nested: holding one never implies another.

- **read** — reads only. The only scope a read-only seat may spend at all.
- **draft** — proposes text. Note this is not read-only: one drafting action
  saves a draft on the deal's timeline.
- **write** — every change that stays inside your company.
- **send** — the four actions that put something on the wire, booking a
  meeting included.
- **enrich** — the one action that fetches from a third party.

A passport with only `read` can read your approval inbox but is refused the
decision. A passport needs `write` to approve most things, and `send` on top of
that where approving puts a message on the wire.

A passport lasts **30 days by default**, at least an hour and at most 90 days.
The token is shown **once** and never again — the app says so: "Copy it now —
you'll only see this token once." Only the hash is stored, so nobody, including
an administrator, can recover it for you.

Revoking takes effect at the agent's next call. So does demoting the colleague
behind it: authority is re-derived every time, so a change binds mid-session
rather than at the next login.

### Letting an agent work overnight on your behalf

A scheduled agent runs while nobody is at a keyboard, so it cannot borrow your
authority from a session you are not in. It has to be given, in advance, by you:
Settings lists every scheduled agent this installation runs, and you answer for
each one — granted, declined, or not yet asked.

Granting **mints your own passport** in the same act. That is the whole point:
the overnight run carries a credential that is yours, bound to you as both the
colleague acted for and the one who granted it, so everything it does is limited
to what you could have done yourself and is attributed to you. Withdrawing the
grant revokes that credential rather than merely unlinking it — the authority
actually ends.

You may see a grant that says you agreed and is still not working. That is
honest rather than broken: a passport expires on its own schedule, and nothing
writes to your answer when it does. The screen tells you the credential behind
the grant is no longer live, and the remedy is to grant it again, which mints a
fresh one.

Nobody can do this on your behalf, and no agent can do it for itself. An agent
that could grant itself standing authority would be deciding its own rights.

### Volume allowances

Each passport gets a fixed allowance per 24-hour window: records read, changes
made, outward calls, and total calls. A fifth counter tracks model tokens spent;
it is advisory and governs nothing.

Two of these behave differently when they run out. Reading and writing are
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

The app's own summary: "Point any MCP-capable agent at your company and
approve the access it asks for. There is nothing to set up first."

When an outside application asks to connect, you get a consent screen that
says plainly "{client} will be able to act in Margince as you, with the access
checked below", names the host the authorization is sent back to, and shows
`read draft write send enrich` as checkboxes, all ticked by default, for you
to untick before approving. You can deny it. There is nothing to set up
first: approving the screen is what creates the connection.

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

