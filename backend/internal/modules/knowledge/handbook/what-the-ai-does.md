# What the AI does, and what it does not

What the AI produces for you, and how to read it. The rules that limit it (the
two tiers, what waits for a human, passports and allowances) are on
[Agents, passports and what they may do](agents-and-passports.md).

## Questions about the AI

### What does the AI do in Margince?
The AI in Margince drafts emails, reads deal documents for deal fields, enriches company records from the web, answers questions from a document set you filed, and ranks your Morning brief overnight. Everything it proposes carries its evidence, and changes to your records either wait as an approval or appear as an automatic change you can switch off in **Settings → Agents**.
Also called: AI features, assistant, copilot, what can Margince AI do.

### Can the AI send email on my behalf?
Inside the app, no: **Draft with AI** in the email composer writes a draft, and only you press **Send**. An outside agent you connected can send as you, immediately, if you gave it the **Send messages** permission. The consent screen says it "sends messages as you, without asking first". The exception is an installation that set a [send floor](agents-and-passports.md#your-installation-can-be-stricter), which stages every agent send as an approval. Every send still needs recorded consent for its purpose. The overnight Morning brief agent can never send.
Also called: auto-send, send without asking, AI emails customers.

### How do I draft an email with AI?
To have Margince draft an email, open the composer from a contact's page (**Email**, or **Write** where several channels exist) and choose **Draft with AI**. The composer is the confirmation: it says "Review and edit the draft. Sending cannot be undone." Review and edit the draft, fill **Reason for contact** when it is asked, then choose **Send**; nothing asks again. The AI never adds a sign-off; your signature is set in **Settings → Account**.
Also called: AI writer, write a reply, compose with AI.

### How do I get the AI to read a document for deal fields?
To fill deal fields from a file, open the company's **Documents** tab, file the document on a deal with **Add document** (choose **A deal**, not **This company**), then choose **Show extracted fields** on that document and **Read this file**. Margince shows each field it found with the passage it read it from; choose **Accept {count} fields** to save them, or **Dismiss**. Nothing is written to the deal until you accept. A document filed against the company offers no reading.
Also called: extract from contract, read a PDF, parse an offer.

### What happens to my mail and enrichment while the AI provider is down?
While an AI provider is down, out of credit or rejecting its key, Margince stops calling it.
Mail questions and company enrichment wait and try again at the next check, without using up their attempts.
That includes the call that first finds the credit gone, the key refused or the host unreachable.
Requests you make yourself fail at once with a message to contact your system administrator.
Your administrator sees the cause under **Settings**, **System health**, in **AI provider status**.
Also called: AI outage, no credit, mail stuck unsure, enrichment stopped.

## What the AI does for you

**Drafting.** The AI writes email drafts (**Draft with AI**). It does not send
them. Your email signature is yours: the app notes "AI drafts never add a
sign-off."

**Reading documents for deal fields.** You can ask the AI to read a file
attached to a deal. It comes back with the fields it can find in that file (deal
name, amount, currency, expected close date), each shown with the passage it
read them from, and staged for you to accept. Nothing is written to the deal
until you press accept. If the file states none of those fields, it says so.
The message reads "AI read this file and found none of the deal fields".
A field it is unsure of is left out and marked "omitted (stated, but not
clearly enough to accept)".

Note that a document filed against **a deal** can be read for deal fields; one
filed against **the company** cannot.

**Enriching a record from the web.** Reading a company's website, matching a
LinkedIn connection, filling in a new account. Every one of these stages a card
instead of writing straight to the record.

**Answering questions about your documents.** Choose **Ask your documents** in
the command palette (⌘K or Ctrl+K), pick a **Document set** and type **Your
question**. See
[Documents and files](documents-and-files.md). It answers only from the set of
documents you filed, and refuses a question that set does not cover.

Only a human can ask a document set. An agent is refused, however wide its
passport. See [Agents, passports and what they may
do](agents-and-passports.md#things-the-ai-is-refused-outright).

**The overnight brief.** The Morning brief looks at your accounts overnight and
ranks what deserves your first hour. If it found nothing, it says so. It runs only if you turned on **Let Margince prepare
the Morning brief overnight** in **Settings → Connections**, and it can read and
write but never send.

Where the overnight pass has something to say about a ranked deal, it writes it
onto the item itself, beside the rank. Every finding names a deal that is already in your queue; the
pass cannot add a deal to your brief by writing about it.

**A deal you dismissed, coming back.** Dismissing a deal keeps it out of every
later brief. It comes back only when something happens on it: a linked activity
after the moment you dismissed it. A returning deal shows the day you dismissed
it and the activity that brought it back.

## Every derived claim carries its evidence

Everything the AI in Margince writes on screen shows where it came from. A generated value
carries the records it was written from, and you can open them. An enriched
field on a contact carries the verbatim snippet it was read from. A number in a
report carries the records it adds up from.

If you disagree with something Margince worked out, you can record that verdict.
The next time Margince works it out, it keeps your correction instead of
overwriting it.

## Who did what: the trust marks

Everywhere a value can be attributed, Margince says who put it there. The trust
marks are:

- "Typed by a person", "Typed by you", "Typed by a buyer", or the colleague's
  name
- "Automated by {agent}", or "Automated by an agent" when the passport has no
  readable name
- "System task {job}", or plain "System task" where the job has no readable
  name: the installation's own housekeeping, such as a scheduled sweep or a
  catch-up run. It is named apart from an agent so you know whether a model
  decided something or Margince did routine work.
- "Via {connector}": it arrived from a connected mailbox
- "Source not recorded": when nothing is known

## Watching work in progress

While the AI in Margince is working for you, you can see it in the agent panel
(**Open agent panel**, under **Running now**). These kinds of work show there:

- your morning brief and the overnight risk sweep
- reading a document, reading a company's site, and scanning an account
- summarising, drafting a reply and drafting an offer
- your weekly review and the learnings behind it
- proposing next steps from a transcript
- building your writing voice

Each shows as queued, running, done, degraded, failed or **stalled**.

Work can also be **deferred** because the company's monthly AI allowance is
spent, or because the AI provider is not answering. Deferred work has not failed.
Work held for the allowance runs when the allowance is raised or the new month
starts. Work held for the provider runs by itself at the provider's next check,
and raising the allowance does not hasten it. See [Settings](settings.md).

"Stalled" means the work has been running unusually long and may have stopped.
The app says: "Reading is taking unusually long and may have
stopped."

Where there is a way out, the same line says it. An account whose read stalled
is read again when you open it again: the open starts a fresh attempt, and the
stalled one gives way to it. A document whose read stalled offers **Read again**
where it showed "Reading this file…", and pressing it does the same.
A read still inside its time is joined instead of restarted, so opening a page
twice, or pressing twice, never reads anything twice.

A run that is waiting on a human is never called stalled. It is waiting, and it
may wait as long as it needs to.

Two limits worth knowing. Work with nobody behind it, such as a nightly sweep,
appears on nobody's list, because there is no one to show it to. And some
tasks only report when they are finished, so you may press "read this site", see
nothing for forty seconds, and then find it already done.

## Where to see what the AI is doing

The **Worklist** on **Home** is the day's shape: what needs a decision, today's meetings,
deals going quiet, promises you made, what ran on its own overnight.

The **Home** brief ranks accounts and shows the factors behind each ranking
(winnability, revenue, timing, momentum, warmth), with the evidence behind them.

The **AI** group in Settings holds four pages: **AI usage**, **AI models**,
**AI call log** and **Automations**.

In **Settings → Agents** you create your own passports, connect MCP clients and
switch **Automatic changes** on or off.

The full audit trail is in **Settings → Audit log**: every action, attributed to
a human, an agent or a connector.
