<!-- prose:plain -->
# What the AI does, and what it does not

What the AI makes for you, and how to read it. The rules that limit it (the
two tiers, what waits for a human, passports and allowances) are on
[Agents, passports and what they may do](agents-and-passports.md).

## Questions about the AI

### What does the AI do in Margince?
The AI in Margince drafts emails, reads deal documents for deal fields, and fills in company records from the web. It answers questions from a document set you filed, and ranks your Morning brief during the night.

Everything it suggests carries its evidence. Changes to your records either wait as an approval, or appear as an automatic change you can switch off in **Settings → Agents**.
Also called: AI features, assistant, copilot, what can Margince AI do.

### Can the AI send email on my behalf?
Inside the app, no: **Draft with AI** in the email composer writes a draft, and only you press **Send**. An outside agent you connected can send as you, at once, if it holds the **Send messages** permission. The screen where you allow that says it "sends messages as you, without asking first".

The one exception is an installation that set a [send floor](agents-and-passports.md#your-installation-can-be-stricter), which turns every agent send into an approval. Every send still needs recorded consent for its purpose. The Morning brief agent that runs during the night can never send.
Also called: auto-send, send without asking, AI emails customers.

### How do I draft an email with AI?
To have Margince draft an email, open the composer from a contact's page and choose **Draft with AI**. The button reads **Email**, or **Write** where there are several channels. The composer is where you confirm: it says "Review and edit the draft. Sending cannot be undone."

Check and edit the draft, fill in **Reason for contact** when it asks, then choose **Send**; nothing asks again. The AI never adds a sign-off; you set your signature in **Settings → Account**.
Also called: AI writer, write a reply, compose with AI.

### How do I get the AI to read a document for deal fields?
To fill deal fields from a file, open the company's **Documents** tab and file the document on a deal with **Add document**. Choose **A deal**, not **This company**. Then choose **Show extracted fields** on that document, and **Read this file**.

Margince shows each field it found with the text it read it from. Choose **Accept {count} fields** to save them, or **Dismiss**. Nothing is written to the deal until you accept. A document filed on the company offers no reading.
Also called: extract from contract, read a PDF, parse an offer.

### What happens to my mail and enrichment while the AI provider is down?
While an AI provider is down, out of credit or refusing its key, Margince stops calling it.
Mail questions and company enrichment wait, and try again at the next check, without using up their tries.
That holds for the call that first finds the credit gone, the key refused or the host out of reach too.

Requests you make yourself fail at once, with a message to contact your system administrator.
Your administrator sees why under **Settings**, **System health**, in **AI provider status**.
Also called: AI outage, no credit, mail stuck unsure, enrichment stopped.

## What the AI does for you

**Drafting.** The AI writes email drafts (**Draft with AI**). It does not send
them. Your email signature is yours: the app notes "AI drafts never add a
sign-off."

**Reading documents for deal fields.** You can ask the AI to read a file
attached to a deal. It comes back with the fields it can find in that file (deal
name, value, currency, expected close date). It shows each one with the text it
read it from, and waits for you to accept. Nothing is written to the deal until
you press accept.

If the file gives none of those fields, it says so. The message reads "AI read
this file and found none of the deal fields". A field it is unsure of is left
out and marked "omitted (stated, but not clearly enough to accept)".

Note that a document filed on **a deal** can be read for deal fields; one filed
on **the company** cannot.

**Filling in a record from the web.** Reading a company's site, matching a
LinkedIn connection, filling in a new company. Every one of these shows a card
for you to check, instead of writing right into the record.

**Answering questions about your documents.** Choose **Ask your documents** in
the command palette (⌘K or Ctrl+K), pick a **Document set** and type **Your
question**. See [Documents and files](documents-and-files.md). It answers only
from the set of documents you filed, and refuses a question that set does not
cover.

Only a human can ask a document set. An agent is refused, whatever its passport
allows. See [Agents, passports and what they may
do](agents-and-passports.md#things-the-ai-is-refused-outright).

**The brief during the night.** The Morning brief looks at your companies during
the night and ranks what most needs your first hour. If it found nothing, it
says so. It runs only if you turned on **Let Margince prepare the Morning brief
overnight** in **Settings → Connections**. It can read and write but never send.

When the night pass has something to say about a ranked deal, it writes it on
the item itself, beside the rank. Every finding names a deal that is already in
your list; the pass cannot add a deal to your brief by writing about it.

**A deal you dismissed, coming back.** Dismissing a deal keeps it out of every
later brief. It comes back only when something happens on it: a linked activity
after the moment you dismissed it. A deal that comes back shows the day you
dismissed it, and the activity that made it come back.

## Every derived claim carries its evidence

Everything the AI in Margince writes on screen shows where it came from. A value
the AI wrote carries the records it was written from, and you can open them. A
field the AI filled on a contact carries the words it was read from, as they
were. A number in a report carries the records it adds up from.

If you do not agree with something Margince worked out, you can record that.
The next time Margince works it out, it keeps what you said instead of writing
over it.

## Who did what: the trust marks

Where Margince can say who put a value there, it does. The trust marks are:

- "Typed by a person", "Typed by you", "Typed by a buyer", or the colleague's
  name
- "Automated by {agent}", or "Automated by an agent" when the passport has no
  name to show
- "System task {job}", or plain "System task" when the job has no name to show.
  This is the installation's own work, such as a planned sweep or a run that
  makes up for lost time. It has a different name from an agent, so you know whether a model
  decided something or Margince did its daily work.
- "Via {connector}": it came from a connected mailbox
- "Source not recorded": when nobody knows where it came from

## Seeing work in progress

While the AI in Margince works for you, you can see it in the agent panel
(**Open agent panel**, under **Running now**). These kinds of work show there:

- your morning brief and the risk sweep during the night
- reading a document, reading a company's site, and looking through a company
- summing up, drafting a reply and drafting an offer
- your weekly review and what it learned
- suggesting next steps from a call's written record
- building your writing voice

Each shows as queued, running, done, degraded, failed or **stalled**.

Work can also be **deferred**, because the company's monthly AI allowance is
used up, or because the AI provider is not answering. Deferred work has not
failed. Work held for the allowance runs when someone sets a higher allowance, or
when the new month starts. Work held for the provider runs by itself at the
provider's next check, and a higher allowance does not make it run any earlier. See
[Settings](settings.md).

"Stalled" means the work has run for a long time and may have stopped. The app
says: "Reading is taking unusually long and may have stopped."

Where there is a way out, the same line says it. When the read of a company
stalled, Margince reads it again the next time you open it. Opening it starts a
new try, and the stalled one gives way to it. A document whose read stalled
offers **Read again** where it showed "Reading this file…", and pressing it does
the same. A read still inside its time limit carries on, and nothing starts it
again. So opening a page twice, or pressing twice, never reads anything twice.

A run that waits on a human is never called stalled. It is waiting, and it may
wait as long as it needs to.

Two limits worth knowing. Work with nobody behind it, such as a sweep during the
night, appears on nobody's list, because there is no one to show it to. And some
tasks only report when they are finished. So you may press "read this site", see
nothing for 40 seconds, and then find it already done.

## Where to see what the AI is doing

The **Worklist** on **Home** is the shape of the day. It shows what needs a
decision, today's meetings, deals going quiet, promises you made, and what ran
on its own during the night.

The **Home** brief ranks companies and shows the reasons behind each rank, with
the evidence behind them. The reasons are how likely a win is, the money, the
timing, how quickly things move, and how warm the contact is.

The **AI** group in Settings holds four pages: **AI usage**, **AI models**,
**AI call log** and **Automations**.

In **Settings → Agents** you create your own passports, connect MCP clients, and
switch **Automatic changes** on or off.

The full record of actions is in **Settings → Audit log**: every action, with the
human, agent or connected system that did it.
