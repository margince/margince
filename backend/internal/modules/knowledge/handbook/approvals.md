# Approvals

When an agent wants to do something it is not allowed to do on its own, it does
not do it. It writes the intention down and puts it in front of a person. That
list of written-down intentions is the **approval inbox**.

This page is about what lands there and how you answer it.

## What a card is

A card in the inbox holds four things:

1. **What is proposed** — the exact change, in full.
2. **The evidence it was formed on** — the records, the passage, the snippet.
3. **How confident** the proposal is — high, medium or low.
4. **Who proposed it** — the agent, shown as "via {verb}" with the action it
   was trying to take.

Nothing has happened yet. The card is a request, not a receipt.

## What stages into the inbox

Below is every kind of card the product can raise, in its own words. Note the
word *can*: which of these you actually see depends on what your agents attempt,
and on whether your installation has set stricter floors. See
[What the AI does](what-the-ai-does.md#what-actually-waits-for-a-human) for
which actions wait by default.

**Records**
- Update a record · Create a record · Archive a record · Merge two records
- Hand a record to an owner · Rename an account · Account stage

**Selling**
- Move a deal forward · Correct a close date · Add a follow-up on a deal
- Promote a lead · Disqualify a lead
- Move a project to its next phase

**Messages**
- Send an email · Send an email to an account · Send a message
- Review a drafted email · Release a stopped message · Book a meeting

**Filing**
- Refile an activity · Refile a conversation · Refile several activities
- Add someone from your mail · Add a person found on the site
- Commit an import

**Learning about an account**
- Fill in a new account · Enrich from the web · Read the company site
- LinkedIn match · Add a next step from a transcript

**Housekeeping**
- Refresh exchange rates · Refresh model prices · Record an automation step
- Let an agent continue

## Deciding

You have three answers.

**Approve.** The proposed change commits, in one transaction that also writes
the audit record. The card comes back showing what it produced.

**Approve edited.** You change the payload first — retype the subject line,
correct a value — and *the edited version is what executes*. Not the original.
This is a real edit, not a comment.

**Reject.** Nothing commits. No record changes. You can give a reason, and the
reason is shared with the person the action was staged for, so a rejection is
not a silent dead end.

A rejection is a decision, not a free action. It demands exactly the same
authority approving does, and it is recorded exactly the same way.

## Who can decide

Two rules, and they matter more than they look.

**You can only decide what you could have done yourself.** The inbox is not the
whole table — it is what *you* may decide. An item whose target you cannot see,
or whose effect you could not perform, is simply absent. It is not listed and
then refused, because listing it would tell you the record exists.

Opening such a card by its link answers "not found", the same as an
out-of-scope record does.

**Nobody releases their own proposal.** An agent may not approve a card its own
credential staged. It may still reject it.

A person's own direct action needs no approval — a human doing the thing
themselves *is* the confirmation.

## Editing, versions and clashes

If the record changed underneath a card while it sat in the inbox, the app
tells you so and offers to re-read rather than letting you approve a proposal
formed against state that no longer exists.

If someone else got there first, you see "Already decided — nothing left to do
here." Deciding twice is not possible; the second attempt is refused rather
than quietly repeated.

## Expiry

**A card expires after 72 hours** if nobody decides it.

Three days rather than one is a deliberate choice. At 24 hours, a proposal
raised on Friday afternoon had auto-rejected before anyone could have seen it,
and the rejection is silent — so the only evidence was work that quietly did
not happen. Three days carries Friday afternoon to Monday morning.

An expired card shows as **Expired**. A pending card shows a countdown:
"expires in {countdown}".

The reasoning behind expiry is that a week-old intention should not be executed
against today's records. It should be proposed again, against the state it can
actually see.

**One kind never expires: a stopped scheduled message.** The message itself is
being held and nothing else will reap it, so the card waits until a person
answers, however long that takes. A card that expired here would leave a
message waiting with nothing asking about it — which is exactly the silent stop
the card exists to prevent.

Individual cards may carry a shorter or longer window than 72 hours where the
thing they are about deserves one — a proposal about a deal closing tomorrow
goes stale sooner than the same proposal about one closing next quarter.

## Bundles: several proposals from one act

Sometimes one action produces several proposals at once. Reading a company's
website, for example, can produce facts about the company *and* people found on
it. Those arrive as a **bundle** and can be decided together.

A bundle is a grouping, not a second thing to have permission over. Each member
is still decided on its own terms — its own verdict, its own audit record, its
own effect. So:

- Deciding a bundle is **not** all-or-nothing. The result reports each member
  separately.
- A member that has expired, or that someone already answered, or whose change
  fails to land, is reported on its own instead of taking the rest down with it.
- Members you could not decide individually are neither shown nor decided, so a
  bundle may report fewer members than it actually holds.

## Worked example: a meeting transcript becomes a task

The most common way a card reaches your inbox without an agent being involved.
Every step here is one you take yourself.

1. Open the **contact** who was in the meeting. Start from the person, not from
   the company: a meeting is with a person, and a company page will ask you who
   was there before it can log one.
2. **Log activity**, and choose **Meeting**.
3. Give it a subject you will recognise later, then tick **This text is a
   transcript** and paste the transcript into the body. The tick matters: it
   routes the text through the normaliser that numbers the lines, and those line
   numbers are what the evidence below points at.
4. **Log**.
5. Open **History**, find the meeting, and click **Read transcript**. Margince
   does not read it on upload — you ask for the read, and this is the step that
   asks.
6. Wait for **Done**. It reports what it found: "{count} next steps waiting for
   your review", or "Read in full. This conversation states no next steps."
7. Go to the **Worklist**, then **Decisions**. A busy queue can bury a new
   proposal, so look for the subject you chose in step 3.
8. Open the card's evidence. It shows the transcript lines the proposal was
   read from, verbatim. Read them before you answer: a proposal is a reading of
   what somebody said, and the lines are how you check the reading.
9. **Approve** to create the task, or **Reject** with a reason. An approved
   proposal becomes an ordinary task on the contact and on their company.

What this does **not** do: it does not send anything, and it does not act
without the read in step 5 and the answer in step 9. A transcript sitting in an
activity has proposed nothing until you ask it to.

## Where you see them

The approval inbox is reachable in its own right, and staged items also surface
where the work is:

- **Worklist → Needs you** counts the decisions waiting on you. When there are
  none it says "Nothing needs a decision."
- A deal page shows "Awaiting your confirmation" when something is staged
  against it.
- Sharing a record that requires approval tells you so at the moment you do it:
  "This share needs approval before it takes effect — it's been queued to the
  approval inbox, not applied yet."

## The point of all this

An approval card is not a speed bump. It is the record of a decision: who
proposed what, on what evidence, who answered, when, and why. That record is
what makes it safe to let an agent work inside your customer data rather than
beside it.

If you approve everything without reading it, you have not made the product
faster. You have only moved where the mistake gets made.
