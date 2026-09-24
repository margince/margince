# What the AI does, and what it does not

This page is what the AI actually produces for you, and how to read it. The
rules that bound it — the two tiers, what waits for a human, passports and
allowances — are on
[Agents, passports and what they may do](agents-and-passports.md).

## Questions about the AI

### What does the AI do in Margince?
The AI in Margince drafts emails, reads deal documents for deal fields, enriches company records from the web, answers questions from a document set you filed, and ranks your Morning brief overnight. Everything it proposes carries its evidence, and changes to your records either wait as an approval or appear as an automatic change you can switch off in **Settings → Agents**.
Also called: AI features, assistant, copilot, what can Margince AI do.

### Can the AI send email on my behalf?
Inside the app, no: **Draft with AI** in the email composer writes a draft, and only you press **Send**. An outside agent you connected can send as you, immediately, if you gave it the **Send messages** permission; the consent screen says it "sends messages as you, without asking first". Every send still needs recorded consent for its purpose. The overnight Morning brief agent can never send.
Also called: auto-send, send without asking, AI emails customers.

### How do I draft an email with AI?
To have Margince draft an email, open the composer from a contact's page (**Email**, or **Write** where several channels exist) and choose **Draft with AI**. The composer is the confirmation: it says "Review and edit the draft. Sending cannot be undone." Review and edit the draft, fill **Reason for contact** when it is asked, then choose **Send**; nothing asks again. The AI never adds a sign-off; your signature is set in **Settings → Account**.
Also called: AI writer, write a reply, compose with AI.

### How do I get the AI to read a document for deal fields?
To fill deal fields from a file, open the company's **Documents** tab, file the document on a deal with **Add document** (choose **A deal**, not **This company**), then choose **Show extracted fields** on that document and **Read this file**. Margince shows each field it found with the passage it read it from; choose **Accept {count} fields** to save them, or **Dismiss**. Nothing is written to the deal until you accept. A document filed against the company offers no reading.
Also called: extract from contract, read a PDF, parse an offer.

## What the AI actually does for you

**Drafting.** The AI writes email drafts (**Draft with AI**). It does not send
them. Your email signature is yours: the app notes "AI drafts never add a
sign-off."

**Reading documents for deal fields.** You can ask the AI to read a file
attached to a deal. It comes back with the fields it can ground in that file — deal
name, amount, currency, expected close date — each shown with the passage it
read them from, and staged for you to accept. Nothing is written to the deal
until you press accept. If the file states none of those fields, it says so
rather than inventing them: "AI read this file and found none of the deal
fields." A field it is unsure of is left out and marked "omitted (stated, but
not clearly enough to accept)".

Note that a document filed against **a deal** can be read for deal fields; one
filed against **the company** cannot.

**Enriching a record from the web.** Reading a company's website, matching a
LinkedIn connection, filling in a new account. Every one of these stages a card
rather than writing straight to the record.

**Answering questions about your documents.** Choose **Ask your documents** in
the command palette (⌘K or Ctrl+K), pick a **Document set** and type **Your
question**. See
[Documents and files](documents-and-files.md). The important property is that
it answers only from the set of documents you filed, and a question that set
does not cover is refused rather than guessed at.

This one is **yours to ask, not an agent's.** Asking a document set is refused
outright to an agent, however wide its passport — see
[Agents, passports and what they may
do](agents-and-passports.md#things-the-ai-is-refused-outright). The reason is the same one that earns the text box in the first
place: a reader of a grounded answer can see which passage each sentence
rests on and go and check it, and an agent acting on that answer unattended
cannot.

**The overnight brief.** The Morning brief looks at your accounts overnight and
ranks what deserves your first hour. If it found nothing, it says so plainly
rather than inventing work. It runs only if you turned on **Let Margince prepare
the Morning brief overnight** in **Settings → Connections**, and it can read and
write but never send.

Where the overnight pass has something to say about a ranked deal, it writes it
onto the item itself, beside the rank — so the reason is in the same place as
the thing it is a reason for, rather than in a second list you have to hold
against the first. Every finding names a deal that is already in your queue; the
pass cannot add a deal to your brief by writing about it.

**A deal you dismissed, coming back.** Waving a deal away holds it out of every
later brief — it does not come back tomorrow because tomorrow is a new day. It
comes back only when something actually happens on it: a linked activity after
the moment you dismissed it. So a returning deal always carries the pair that
explains it — the day you dismissed it, and the activity that brought it back.
That is not the software guessing why it is showing you something twice. It is
the rule that put it back, stated: no activity, no return.

## Every derived claim carries its evidence

Nothing the AI in Margince writes on screen is presented as a bare fact. A generated value
carries the records it was written from, and you can open them. An enriched
field on a contact carries the verbatim snippet it was read from. A number in a
report carries the rows it reconciles to.

If you disagree with something the system derived, you can record that verdict.
Your correction is what the next re-derivation has to respect — it is not just a
dismissal that gets overwritten next time.

## Who did what: the trust marks

Everywhere a value can be attributed, Margince says who put it there. The trust
marks are:

- "Typed by a person", "Typed by you", "Typed by a buyer", or the colleague's
  name
- "Automated by {agent}", or "Automated by an agent" when the credential
  carries no readable name, because printing an opaque id at you would not help
- "System task {job}", or plain "System task" where the job has no readable
  name: the installation's own housekeeping, such as a scheduled sweep or a
  backfill. Deliberately named apart from an agent, because "a model decided
  this" and "the system did its housekeeping" are different answers to the
  question "who do I ask about this?"
- "Via {connector}": it arrived from a connected mailbox
- "Source not recorded": when honestly nothing is known

## Watching work in progress

While the AI in Margince is working for you, you can see it in the agent panel
(**Open agent panel**, under **Running now**). Twelve kinds of work are
narrated: your morning brief, the overnight risk sweep, reading a document,
reading a company's site, summarising, scanning an account, drafting a reply,
drafting an offer, your weekly review, the learnings behind it, proposing next
steps from a transcript, and building your writing voice.

Each shows as queued, running, done, degraded, failed — or **stalled**.

There is a seventh state those six do not name: work can be **deferred** because
the company's monthly AI allowance is spent. That is not a failure and not a
stall — the request is kept whole, with its original authority and attempt
limits, and a sweep picks it up when the allowance is raised or the month rolls.
See [Settings](settings.md).

"Stalled" means the work has been running unusually long and may have stopped.
The app says exactly that: "Reading is taking unusually long and may have
stopped."

Where there is a way out, the same line says it. An account whose read stalled
is read again when you open it again: the open starts a fresh attempt, and the
stalled one gives way to it. A document whose read stalled offers **Read again**
where it showed "Reading this file…", and pressing it does the same.
A read still inside its time is joined rather than restarted, so opening a page
twice, or pressing twice, never reads anything twice.

It is worked out fresh each time you look, and never written down. That is
deliberate. Nothing has to remember to mark it, which is what stops a job that
died halfway from being shown as working forever.

A run that is waiting on a *human* is never called stalled. It is waiting, which
is a different thing, and it may wait as long as it needs to.

Two limits worth knowing. Work with nobody behind it — a nightly sweep — reaches
nobody's list, not by choice but because there is no one to show it to. And some
tasks only report when they are finished, so you may press "read this site", see
nothing for forty seconds, and then find it already done.

## Where to see what the AI is doing

The **Worklist** on **Home** is the day's shape: what needs a decision, today's meetings,
deals going quiet, promises you made, what ran on its own overnight.

The **Home** brief ranks accounts and shows the factors behind each ranking —
winnability, revenue, timing, momentum, warmth — with the evidence rows behind
them.

The **AI** group in Settings holds four pages: **Models and routing**,
**Automations**, **AI usage** and **Model calls**. **Settings → Agents** is where
you mint your own passports, connect MCP clients and switch **Automatic
changes** on or off. **Settings → Audit log** holds the full audit trail: every action,
attributed to a human, an agent or a connector.
