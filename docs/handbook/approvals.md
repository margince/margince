# Approvals

An approval in Margince is a change an agent proposed but was not allowed to
make on its own. The agent does not do it: it writes the intention down and puts
it in front of a human, who accepts, edits or rejects it. The product calls the
waiting items **Approvals**.

This page is about what lands there and how you answer it.

## Answering approvals

### How do I approve an action in Margince?
To approve a proposed change in Margince, open **Home**, show the **Worklist**, find the approval row and choose **Decide**, then **Accept**.
1. Open **Home** and choose **Show Worklist** if it is hidden.
2. Set **Work type** to **Approvals** to see only approvals.
3. On the row, choose **Decide**. The **Your decision** panel opens.
4. Read the proposal and its evidence, then choose **Accept**.
The toast says **Applied**, with **Undo on record** to reverse it from the record's history.
Also called: confirm, accept, sign off, OK an agent action.

### Where are my approvals?
Margince has no separate approvals inbox. Approvals waiting on you appear in three places:
1. **Home → Worklist**: each approval is a row with **Decide**; set **Work type** to **Approvals** to filter.
2. The agent panel (**Open agent panel**), in its **Approvals** section.
3. On a company page, **Pending approvals**, with **Review {count} pending**; a deal page shows **Awaiting approval**.
When nothing waits, the Worklist says "Nothing is waiting on you."
Also called: approval queue, pending decisions, inbox, to review.

### How do I edit a proposal before approving it?
To change a proposal before it runs, open it with **Decide**, choose **Edit**, change the values (for a draft email, the **Subject** and **Message**), then choose **Approve edited**. The edited version is what executes, not the original.
Also called: amend, modify, correct an agent draft.

### How do I reject a proposal?
To turn a proposal down, open it with **Decide** and choose **Reject**. In the app this is one press with no reason form. Nothing commits and no record changes. A rejection needs the same authority as approving it, and it is recorded the same way.
Also called: decline, deny, dismiss an agent action.

### What happens if an approval expires?
An approval in Margince expires after **72 hours** if nobody decides it. Nothing is applied; the card shows **Expired**, and the agent has to propose again against the current state of the record. A pending card shows "expires in {countdown}". One kind never expires: a stopped scheduled message waits until somebody answers.
Also called: approval timed out, missed an approval, stale approval.

### Who approves agent actions?
An agent action in Margince is approved by the colleague the agent acts for, or by a colleague who could have made the same change themselves. You only see approvals whose target you can see and whose effect you could perform. No agent may approve its own proposal or one staged for somebody else. An administrator has no override beyond their own permissions.
Also called: approver, who signs off, permission to approve.

### How do I stop Margince making automatic changes?
To make Margince ask before changing close dates, company names or lifecycle stages, open **Settings → Agents** and turn off the matching switch under **Automatic changes**. The switches apply to your work only. A switched-off kind comes back to your Worklist as an approval.
Also called: autopilot, auto-apply, stop the AI editing my records.

### Can I undo what the AI changed?
Yes. To undo a change the AI or an agent made in Margince, press **Undo** on its row under **Handled for you** in the Worklist, or on the entry in the record's history.
1. **Home** → **Show Worklist**: **Handled for you** lists the last 24 hours of actions taken for you, with **Undo** on deals.
2. For a company name, a lifecycle stage or anything older, choose **Undo** in the record's history: **History** → **Changes** on a contact, lead or deal, **More actions** → **Full history** on a company.
Only a human can undo; an agent cannot.
Also called: revert the AI, roll back an agent edit, reverse an automatic change.

## What an approval card holds

An approval card in Margince holds four things:

1. **What is proposed**: the exact change, in full.
2. **The evidence it was formed on**: the records, the passage, the snippet.
3. **How confident** the proposal is: high, medium or low.
4. **Who proposed it**: the agent, tagged "Automated by {agent}", with the
   action it was trying to take.

Nothing has happened yet. The card is a request, not a receipt. **Approval
detail** shows when it was **Asked** and **Decided**, and **Technical details**
shows the raw proposal.

## What can become an approval

Below is every kind of approval the product can raise. Note the word *can*:
which of these you actually see depends on what your agents attempt, and on
whether your installation has set stricter floors. See
[Agents, passports and what they may do](agents-and-passports.md#what-actually-waits-for-a-human)
for which actions wait by default.

**Records**
- Update a record · Create a record · Archive a record · Merge two records
- Hand a record to an owner · Rename an account · Account stage
- Fold one tag into another · Create a contact from a card

**Selling**
- Move a deal forward · Move a deal to the next stage · Correct a close date
- Add a follow-up on a deal
- Promote a lead · Disqualify a lead · Reverse a lead promotion
- Move a project to its next phase

**Messages**
- Send an email · Send an email to an account · Send a message
- Review a drafted email · Decide a refused email · Release a stopped message
- Book a meeting

**Filing**
- Refile an activity · Refile a conversation · Refile several activities
- Add someone from your mail · Add a contact found on the site
- Commit an import

**Learning about an account**
- Fill in a new account · Enrich from the web · Read the company site
- LinkedIn match · Add a next step from a transcript

**Housekeeping**
- Refresh exchange rates · Refresh model prices · Record an automation step
- Let an agent continue

## Automatic changes: what answers itself

Automatic changes are the exception to everything below: **not every proposal
waits.**

Three kinds of change can apply on their own, without an approval being
decided: **Close dates**, **Company names** and **Lifecycle stages**. They land
under **Handled for you** on the Worklist rather than as an approval.

This is per seat, not company-wide, and it is **on by default**. The switches
are at **Settings → Agents**, under **Automatic changes**, which says: "Automatic
changes start on. Existing settings are kept. Each switch applies to your work,
not the whole team."

Beside each switch is your own track record with that kind of proposal: "So
far: {clean} approved as proposed, {edited} approved after edits, {rejected}
rejected." The grain is deliberate: approving fourteen close-date confirmations
is evidence about close dates and none at all about outbound mail, so each kind
earns its own standing.

Turning a switch off sends that kind back to your approvals, where the rest of
this page applies.

## Deciding: accept, edit or reject

An approval has three answers.

**Accept.** The proposed change commits, in one transaction that also writes
the audit record. The card comes back showing what it produced, with **Undo on
record**.

**Approve edited.** You change the payload first (retype the subject line,
correct a value) and *the edited version is what executes*. Not the original.
This is a real edit, not a comment.

**Reject.** Nothing commits. No record changes. The app asks for no reason; the
API accepts one, and a reason given is kept on the decision's audit record.

A rejection is a decision, not a free action. It demands exactly the same
authority approving does, and it is recorded exactly the same way.

## Who can decide an approval

Two rules decide who may answer an approval, and they matter more than they
look.

**You can only decide what you could have done yourself.** Your approvals are
not the whole table: they are what *you* may decide. An item whose target you
cannot see, or whose effect you could not perform, is simply absent. It is not
listed and then refused, because listing it would tell you the record exists.

Opening such a card by its link answers "not found", the same as an
out-of-scope record does.

**Nobody releases their own proposal.** An agent may not approve a card its own
credential staged. It may still reject it.

A colleague's own direct action needs no approval: a human doing the thing
themselves *is* the confirmation.

## Editing, versions and clashes

If the record changed underneath an approval while it waited, the app says
"This record changed after it was staged. Stage it again before deciding." and
offers **Reload**, rather than letting you approve a proposal formed against
state that no longer exists.

If someone else got there first, you see "Already decided. Nothing left to do."
Deciding twice is not possible; the second attempt is refused rather than
quietly repeated.

## Expiry

**An approval expires after 72 hours** if nobody decides it.

Three days rather than one is a deliberate choice. At 24 hours, a proposal
raised on Friday afternoon had auto-rejected before anyone could have seen it,
and the rejection is silent, so the only evidence was work that quietly did not
happen. Three days carries Friday afternoon to Monday morning.

An expired card shows as **Expired**. A pending card shows a countdown:
"expires in {countdown}".

The reasoning behind expiry is that a week-old intention should not be executed
against today's records. It should be proposed again, against the state it can
actually see.

**One kind never expires: a stopped scheduled message.** The message itself is
being held and nothing else will reap it, so the card waits until somebody
answers, however long that takes. A card that expired here would leave a
message waiting with nothing asking about it, which is exactly the silent stop
the card exists to prevent.

Individual cards may carry a shorter or longer window than 72 hours where the
thing they are about deserves one: a proposal about a deal closing tomorrow
goes stale sooner than the same proposal about one closing next quarter.

## Bundles: several proposals from one act

A bundle is several proposals produced by one action. Reading a company's
website, for example, can produce facts about the company *and* contacts found
on it. Those arrive as a **bundle** and can be decided together.

A bundle is a grouping, not a second thing to have permission over. Each member
is still decided on its own terms: its own verdict, its own audit record, its
own effect. So:

- Deciding a bundle is **not** all-or-nothing. The result reports each member
  separately.
- A member that has expired, or that someone already answered, or whose change
  fails to land, is reported on its own instead of taking the rest down with it.
- Members you could not decide individually are neither shown nor decided, so a
  bundle may report fewer members than it actually holds.

## Worked example: a meeting transcript becomes a task

Turning a meeting transcript into tasks is the most common way an approval
reaches you without an agent being involved. Every step here is one you take
yourself.

1. Open the **contact** who was in the meeting, not the company: a meeting is
   with a contact, and a company page will ask you who was there before it can
   log one.
2. **Log activity**, and choose **Meeting**.
3. Give it a **Subject** you will recognise later, then tick **This text is a
   transcript** and paste the transcript into the body. The tick matters: it
   routes the text through the normaliser that numbers the lines, and those line
   numbers are what the evidence below points at.
4. **Log**.
5. Open **History** and find the meeting. Margince queues a reading when you
   log a transcript. If no reading is underway or complete, click **Read
   transcript** to request one.
6. Wait for **Done**. It reports what it found: "{count} next steps awaiting
   review", or "Transcript read in full. No next steps found."
7. Go to **Home → Worklist** and set **Work type** to **Approvals**. A busy queue
   can bury a new proposal, so look for the subject you chose in step 3.
8. Choose **Decide** and open the evidence. It shows the transcript lines the
   proposal was read from, verbatim. Read them before you answer: a proposal is
   a reading of what somebody said, and the lines are how you check the reading.
9. **Accept** to create the task, or **Reject**. An accepted proposal becomes an
   ordinary task on the contact and on their company.

What this does **not** do: it does not send anything or create tasks without
your approval. Reading the transcript proposes next steps; accepting a proposal
is what creates its task.

## Where approvals show up

Staged items surface where the work is:

- **Home → Worklist**, filtered by **Work type → Approvals**. When nothing waits
  it says "Nothing is waiting on you."
- The agent panel's **Approvals** section.
- A company page's **Pending approvals** card, and a deal page's **Awaiting
  approval** panel.
- Sharing a record that requires approval tells you so at the moment you do it:
  "This share takes effect only after approval. It is not applied yet."

## The point of all this

An approval card is not a speed bump. It is the record of a decision: who
proposed what, on what evidence, who answered, when, and why. That record is
what makes it safe to let an agent work inside your customer data rather than
beside it.

If you approve everything without reading it, you have not made the product
faster. You have only moved where the mistake gets made.
