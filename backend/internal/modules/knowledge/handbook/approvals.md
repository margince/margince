<!-- prose:plain -->
# Approvals

An approval in Margince is a change an agent proposed but was not allowed to
make on its own. The agent does not do it. It writes down what it means to do
and puts it before a human, who accepts, edits or rejects it. The product
calls the waiting items **Approvals**.

## Answering approvals

### How do I approve an action in Margince?
To approve a proposed change in Margince, open **Home** and show the **Worklist**. Find the approval row and choose **Decide**, then **Accept**.
1. Open **Home** and choose **Show Worklist** if it is hidden.
2. Set **Work type** to **Approvals** to see only approvals.
3. On the row, choose **Decide**. The **Your decision** panel opens.
4. Read the proposal and its evidence, then choose **Accept**.

If it edited a record, the **Applied** message offers **Undo on record** to reverse it from the record's history; a sent email has no undo.
A new contact, company or deal can be taken back with **Undo** on its "Created" entry in the record's history, which archives it.
Also called: confirm, accept, sign off, OK an agent action.

### Where are my approvals?
Margince has no approvals inbox of its own. Approvals waiting on you appear in three places:
1. **Home → Worklist**: each approval is a row with **Decide**; set **Work type** to **Approvals** to filter.
2. The agent panel (**Open agent panel**), in its **Approvals** section.
3. On a company page, **Pending approvals**, with **Review {count} pending**; a deal page shows **Awaiting approval**.
When nothing waits, the Worklist says "Nothing is waiting on you."
Also called: approval queue, pending decisions, inbox, to review.

### How do I edit a proposal before approving it?
To change a proposal before it runs, open it with **Decide** and choose **Edit**. Change the values (for a draft email, the **Subject** and **Message**), then choose **Approve edited**. The edited version is what runs, not the original.
Also called: amend, modify, correct an agent draft.

### How do I reject a proposal?
To turn a proposal down, open it with **Decide** and choose **Reject**. In the app this is one press with no reason form. Nothing is applied and no record changes. Rejecting needs the same authority as approving it, and it is recorded the same way.
Also called: decline, deny, dismiss an agent action.

### What happens if an approval expires?
An approval in Margince expires after **72 hours** by default if nobody decides it; a single card can carry a shorter or longer window.
Nothing is applied. The card shows **Expired**, and the agent has to propose again against the current state of the record.
A pending card shows "expires in {countdown}". One kind never expires: a stopped scheduled message waits until somebody answers.
Also called: approval timed out, missed an approval, stale approval.

### Who approves agent actions?
An agent action in Margince is approved by the colleague the agent acts for, or by a colleague who could have made the same change. You only see approvals whose target you can see and whose effect you could carry out. No agent may approve a proposal staged for somebody else, or a send it proposed itself. An administrator has no override past their own permissions.
Also called: approver, who signs off, permission to approve.

### How do I stop Margince making automatic changes?
To make Margince ask before changing close dates, company names or lifecycle stages, open **Settings → Agents**. Turn off the matching switch under **Automatic changes**. The switches apply to your work only. A kind you switch off comes back to your Worklist as an approval.
Also called: autopilot, auto-apply, stop the AI editing my records.

### Can I undo what the AI changed?
Yes. To undo a change the AI or an agent made in Margince, press **Undo** on its row under **Handled for you** in the Worklist. You can also press it on the entry in the record's history.
1. **Home** → **Show Worklist**: **Handled for you** lists the last 24 hours of actions taken for you, with **Undo** on deals.
2. For a company name, a lifecycle stage or anything older, choose **Undo** in the record's history.
   That is **History** → **Changes** on a contact, lead or deal, and **More actions** → **Full history** on a company.
Only a human can undo; an agent cannot.
Also called: revert the AI, roll back an agent edit, reverse an automatic change.

## What an approval card holds

An approval card in Margince holds four things:

1. **What is proposed**: the change itself, in full.
2. **The evidence it was formed on**: the records, the passage, the part of the text.
3. **How confident** the proposal is: high, medium or low.
4. **Who proposed it**: the agent, tagged "Automated by {agent}", with the
   action it was trying to take.

Nothing has happened yet. The card is a request, not a receipt. **Approval
detail** shows when it was **Asked** and **Decided**, and **Technical details**
shows the full proposal as it was sent.

## What can become an approval

Below is every kind of approval the product can raise. Which of these you see
depends on what your agents try, and on whether your installation has made
more actions wait. See
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

**Account research**
- Fill in a new account · Enrich from the web · Read the company site
- LinkedIn match · Add a next step from a transcript

**Other**
- Refresh exchange rates · Record an automation step
- Let an agent continue

## Automatic changes: what answers itself

Automatic changes are the exception to everything below: some proposals do not
wait.

Three kinds of change can apply on their own, without an approval being
decided: **Close dates**, **Company names** and **Lifecycle stages**. They land
under **Handled for you** on the Worklist rather than as an approval.

This is set per seat, and it is **on by default**. The switches
are at **Settings → Agents**, under **Automatic changes**, which says: "Automatic
changes start on. Existing settings are kept. Each switch applies to your work,
not the whole team."

Each switch shows how you have answered that kind of proposal so far.
It reads: "So far: {clean} approved as proposed, {edited} approved after edits, {rejected}
rejected." Each kind keeps its own record, because approving many close date
changes says nothing about mail you send out.

Turning a switch off sends that kind back to your approvals, where the rest of
this page applies.

## Deciding: accept, edit or reject

An approval has three answers.

**Accept.** Margince applies the change and records it in the audit log. When
it changed a record, the message that appears offers **Undo on
record**, which opens that record's history to reverse it. A kind that sends
mail or names no record has nothing to put back, so it offers no undo. A new
contact, company or deal is taken back from its "Created" entry in the record's
history.

**Approve edited.** You change the proposal first (type the subject line again,
correct a value), and *the edited version is what runs*, in place of the
original.

**Reject.** Nothing is applied and no record changes. The app asks for no
reason. When an agent rejects with a reason, the reason is kept in the audit
log.

Rejecting needs the same authority as approving, and it is recorded the same
way.

## Who can decide an approval

Two rules decide who may answer an approval.

**You decide only what you could do yourself.** Your approvals list shows what
*you* may decide. An item whose target you cannot see, or whose effect you
could not carry out, is not listed at all. Listing it would tell you the record
exists.

Opening such a card by its link answers "not found", the same as a record
outside your scope does.

**Agents and their own proposals.** On your
word, an agent may approve a proposal it staged for you only in a narrow case.
The change must be one that would have applied by itself if a human had not
edited the record first. It may not close a deal, relink, merge tags,
change what a field means, send anything, or do anything your installation
requires a human for. And it must happen in a conversation, never on a
schedule. The decision is recorded as yours, given through that agent.

Everything else it proposed is yours to release here, or through a passport you
made by hand for that purpose. It may always reject its own proposal.

A colleague's own action needs no approval: someone doing the thing is the
confirmation.

## When something changed first

The record may change while an approval waits. Then the app says
"This record changed after it was staged. Stage it again before deciding." and
offers **Reload**. So you never approve a proposal based on an older version of
the record.

If someone else answered first, you see "Already decided. Nothing left to do."
Nobody can decide it a second time; that try is refused.

## Expiry

**An approval expires after 72 hours** by default if nobody decides it.

Three days lets a proposal raised on Friday afternoon wait until Monday
afternoon.

An expired card shows as **Expired**. A pending card shows how long is left:
"expires in {countdown}".

Expiry exists because a plan from last week should not run against today's
records. The agent proposes again, against the record as it is now.

**One kind never expires: a stopped scheduled message.** The message is held
until somebody answers, no matter how long that takes. If the card expired, the
message would stay stopped with nothing asking about it.

A single card may carry a shorter or longer window than 72 hours where the
thing it is about needs one. A proposal about a deal closing this week goes
stale sooner than the same proposal about one closing next quarter.

## Bundles: several proposals from one act

A bundle is several proposals made by one action. Reading a company's website,
for example, can make facts about the company *and* contacts found on it. Those
arrive as a **bundle** and can be decided together.

A bundle only groups proposals; it needs no permission of its own. Each member
is still decided on its own terms: its own verdict, its own audit record, its
own effect. So:

- Deciding a bundle is not all or nothing. Margince reports each member on
  its own.
- A member may have expired, or someone may have answered it, or its change may
  fail to land. Then it is reported on its own, and the rest still go through.
- Members you could not decide one by one are neither shown nor decided. So a
  bundle may report only some of the members it holds.

## Worked example: a meeting transcript becomes a task

Turning a meeting transcript into tasks is the most common way an approval
reaches you without an agent taking part. Every step here is one you take
yourself.

1. Open the **contact** who was in the meeting, not the company. A meeting is
   with a contact, and a company page will ask you who was there first.
2. **Log activity**, and choose **Meeting**.
3. Give it a **Subject** you will know again later. Then tick **This text is a
   transcript** and paste the transcript into the body.
   The tick matters: it numbers the transcript lines, and the evidence below
   points at those numbers.
4. **Log**.
5. Open **History** and find the meeting. Margince queues a reading when you
   log a transcript. If no reading is running or done, click **Read
   transcript** to ask for one.
6. Wait for **Done**. It reports what it found: "{count} next steps awaiting
   review", or "Transcript read in full. No next steps found."
7. Go to **Home → Worklist** and set **Work type** to **Approvals**.
   A full queue can hide a new proposal, so look for the subject you chose in step 3.
8. Choose **Decide** and open the evidence. It shows the transcript lines the
   proposal was read from, word for word.
   Read them before you answer. A proposal is a reading of what somebody said,
   and the lines are how you check it.
9. **Accept** to create the task, or **Reject**. An accepted proposal becomes an
   ordinary task on the contact and on their company.

It does not send anything or create tasks without your approval. Reading the
transcript proposes next steps; accepting a proposal is what creates its task.

## Where approvals show up

Staged items show up where the work is:

- **Home → Worklist**, filtered by **Work type → Approvals**. When nothing waits
  it says "Nothing is waiting on you."
- The agent panel's **Approvals** section.
- A company page's **Pending approvals** card, and a deal page's **Awaiting
  approval** panel.
- Sharing a record that requires approval tells you so at the moment you do it:
  "This share takes effect only after approval. It is not applied yet."
