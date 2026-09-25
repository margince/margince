# What Margince is

Margince is a CRM — a place to keep track of the contacts you sell to, the
companies they work for, the deals you are working on, and the work you deliver
afterwards.

It is different from most CRMs in three ways, and the differences are the reason
to use it.

### What is Margince?
Margince is a CRM: one place for your contacts, the companies they work for, the leads you are qualifying, the deals you are working on, and the projects you deliver afterwards.
It differs from other CRMs in three ways. Your AI agents work inside it, under your own permissions. It fills itself in from your connected mailbox and calendar. And it says plainly when it does not know something, rather than guessing.
Everything is reached from the sidebar on the left, or from the command palette with ⌘K (Ctrl+K).
Also called: Margince CRM, Gradion CRM.

## 1. Your AI agents work inside it, not beside it

In Margince, AI agents work inside the CRM, not beside it. Most CRMs bolt AI on as a panel that summarises what you typed in yourself.

In Margince, an AI agent connects to your company properly and gets a set of
governed tools. It can look things up, draft replies, read a document for you,
enrich a company, move a deal along. Everything it does is recorded and
attributed.

The governing rule is one sentence:

> An agent can do what the colleague behind it could do unaided — and nothing
> more. It is checked against that colleague on every call.

An agent has no identity of its own. It acts on behalf of a colleague, using a
credential they minted and can revoke. If you cannot see a record, neither
can your agent. If you are not allowed to do something, it cannot do it for you.

And there is a short list of things it can never do at all, however it is
configured — chief among them: **an agent never releases a proposal that is not
its own business.** It may not approve the card its own credential staged, nor
one staged for a different colleague. It may answer a card staged for the
colleague it acts for, which is exactly what they could have answered
themselves; and it may always reject its own proposal.

[What the AI does, and what it does not →](what-the-ai-does.md)

## 2. It fills itself in

Margince fills itself in from your mail and calendar. You should not have to type into a CRM what you already wrote in an email.

Connect your mailbox and your calendar, and Margince files what arrives against
the right contacts, companies, deals and projects — by exact rules, not by
guesswork. Where the rules cannot decide, it asks you rather than picking.

[How conversations get in →](capture.md)

## 3. It tells you the truth about itself

Margince telling the truth about itself is the least obvious property and, over time, the most useful one.

Margince is unusually careful to distinguish between "no" and "I do not know",
and it says which one it means:

- Asked a question its documents do not cover, it answers **"Not covered by this
  set"** rather than composing something plausible.
- A Worklist with nothing in it says so: "Nothing is waiting on you." An empty
  list is the answer, not an omission.
- A field with no edit history says: "Set at creation and never changed. The
  audit log records no edits." An empty history is honest, not a gap.
- A count taken from a partial read says it was partial, so you do not read a
  blank cell as a fact.
- Where a value has no known source, it says "source not recorded" instead of
  leaving it blank.

Every generated sentence carries the records it was written from. Every enriched
field carries the passage it was read from. Every number in a report reconciles
to rows you can open.

You are meant to be able to check it. That is the design.

## What it holds

Margince holds five kinds of record:

- **Contacts** — the individuals you sell to
- **Companies** — the companies they work for
- **Leads** — prospects, kept deliberately apart from contacts
- **Deals** — pieces of business moving toward won or lost
- **Projects** — the work itself, which starts during the deal and outlives it

Plus the timeline of emails, calls, meetings, notes and tasks that runs through
all of them, and the documents that accumulate along the way.

[What each one is, and how they connect →](records.md)

## Getting around

The Margince sidebar is the list of screens down the left of the app. **Home**
sits on its own at the top. Below it the screens are grouped in three:

- **Records** — **Contacts**, **Companies**, **Leads**, **Deals**
- **Work** — **Projects**, **Filters and views**
- **Intelligence** — **Analytics**

Settings is not in the sidebar: it is in the account menu at the top right. The
**Deals** row is the pipeline board; there is no separate Pipeline row. The
Worklist has no row of its own either; you open it from Home.

You will notice no counters or badges on the sidebar. That is deliberate: the
queues that would carry them are lanes inside the Worklist, which reports its
own numbers on the page rather than nagging from the edge of the screen.

### How do I get around the app?
To get around Margince, use the sidebar on the left or press ⌘K (Ctrl+K on Windows and Linux) to open the command palette and type where you want to go.
1. Choose a screen in the sidebar: **Home**, then **Contacts**, **Companies**, **Leads**, **Deals**, **Projects**, **Filters and views** and **Analytics**.
2. Open **Settings** from the account menu at the top right.
3. To hide the sidebar, choose **Collapse sidebar**, or press ⌘B (Ctrl+B).
On a phone the sidebar becomes a bottom bar with **Home**, **Contacts**, **Deals**, the agent in the centre, and **More** for everything else.
Also called: navigation, menu, nav bar.

### Where is something in the sidebar?
Each Margince screen sits in one place in the sidebar, and a few are reached another way.
- Contacts, companies, leads and deals: the **Records** group.
- Projects, and saved filters and exports: the **Work** group, under **Projects** and **Filters and views**.
- Reports and the forecast: **Analytics**, under **Intelligence**.
- The Worklist, tasks and approvals: **Home**.
- The pipeline: the **Deals** row, which is the board.
- Settings: the account menu at the top right, not the sidebar.
- Scheduled messages, offers and search results: no sidebar row. Open them from the command palette (⌘K).

### How do I search for something?
To search Margince, click the search field **Search or ask Margince** in the top bar, or press ⌘K (Ctrl+K), and start typing.
1. Type a name, company, deal or any other word. Matching records appear as you type, each marked **Record**.
2. Choose a result to open it, or choose **See all results for “…”** to open the full **Search results** page.
3. On that page, **Show only** narrows the results to one kind.
If nothing matches, the palette says **No matches.** If record search fails, it says **Search failed** and the screen commands still work.
Also called: find, look up, global search.

**Search results** are grouped by kind: Contacts, Companies, Deals, Leads,
Projects, Activities, Products, Offer templates and Tags. A result that came
from a connected system is marked **From a connected system**, rather than
looking like something somebody here typed.

### What is the command palette?
The command palette is the Margince box that finds any screen, action, setting or record from one place. It opens over the page you are on, with the placeholder **Search or ask Margince**.
Each row carries a badge: **Screen** for a sidebar screen, **Action** for something it does (**New deal**, **Read a company**, **Booking page**), and **Record** for a record that matches what you typed.
It also lists every settings page you may open, and **Scheduled messages**. It knows other words for a page: "price" finds **Products and offers**, "password" finds **Account**.
Its first row is always **Ask your documents**.

### How do I open the command palette?
To open the Margince command palette, press ⌘K on a Mac or Ctrl+K on Windows and Linux, from any screen.
You can also click the search field **Search or ask Margince** in the top bar.
Press ⌘K again, or Esc, to close it.
Also called: quick search, command menu, Cmd K, Ctrl K, keyboard shortcut.

### Where is the handbook, and how do I ask it a question?
The Margince handbook is inside the app. To ask it a question, open the command palette with ⌘K (Ctrl+K) and choose **Ask your documents**, the first row.
1. The **Ask your documents** dialog opens over the page, carrying whatever you had typed.
2. **Document set** starts on your company's default set, which is the **Margince handbook** unless an administrator chose another. Change it to ask a different set.
3. Type in **Your question**, in your own words, and choose **Ask**.
The answer quotes and cites the handbook. A question it does not cover gets **Not covered by this set**, not a guess.
Also called: help, FAQ, knowledge base, ask Margince.

### How do I ask Margince a question about a company?
To ask about one company, open the company and use its **Ask about this company** panel: **Ask Margince** offers fixed questions — **What is open here?**, **Prepare for a meeting** and **What changed recently?** — answered from the records you can access. It takes no free text; for a question in your own words about how Margince works, use **Ask your documents** (⌘K).
Also called: company summary, meeting prep, what's going on with this account.

### Where is the Ask box?
The Margince Ask box is called **Ask your documents**, and it is the first row of the command palette: press ⌘K (Ctrl+K) or click **Search or ask Margince** in the top bar.
It answers only from one document set: the **Margince handbook** that ships with every installation, or a set your company filed under Settings → **Knowledge**.
On a company page there is a second, separate panel, **Ask about this company**, with its own **Ask Margince** questions about that one company.
If the dialog says **No document sets**, there is nothing filed to search yet.

## Home and the Worklist

Home is where your day starts, and the door to the **Worklist** — the work that
waits on a human: decisions to answer, tasks to finish, duplicates to merge,
today's meetings, deals going quiet, promises you made, and what ran on its own
overnight. Open it on Home with **Show Worklist**. When there is nothing, the
Worklist says "Nothing is waiting on you." rather than showing you an empty grid.

## Words this handbook uses

**Company** — the word does double duty, and the screen always makes clear
which is meant. Your *own* company is the tenant: the whole of your data in
Margince, and what a setting marked "Company" changes. A *company record* is
someone you do business with, listed under Companies.

**Installation** — the running deployment. Whoever operates it decides things
like upload limits and whether sends need confirming. One installation serves
one company.

**Contact** — an individual you do business with. One word everywhere: the
screen, the address bar and the data behind them all say contact.

**Passport** — the credential a colleague mints so an AI agent can act as them.

**Staged** — an action that has not happened, and is waiting for a human to
decide.

**Archived** — removed from the live lists, still there. Almost everything in
Margince is archived rather than deleted. What actually destroys data is a short,
explicit list.

## Other words for the same thing

Margince uses one name for each thing. If you know it by another word, this
glossary gives the Margince name.

An opportunity is called a **deal** in Margince. Deals are in the **Deals** row of the sidebar, and the pipeline is the board on that screen.

An organisation, an organisation record, an account or a customer account is called a **company** in Margince. To add a new organisation record, open **Companies** in the sidebar, choose **New company**, enter the **Company name** and choose **Create**.

A customer or a client is a **company** you do business with, and the individuals there are **contacts**. Margince has no separate customer or client record.

A prospect is called a **lead** in Margince. A lead is kept apart from contacts until you qualify it. Leads are in the **Leads** row of the sidebar.

A reseller, a referral partner or a channel partner is called a **partner** in Margince: a company with partner terms, set up from the company's **More actions** → **Set up partner program**. A referral fee is its **commission**.

A proposal or a quote is called an **offer** in Margince. You create an offer from the **Offers** panel on a deal, and the price list behind it is called **Products and offers** in Settings.

A teammate or a colleague who signs in is called a **user** in Margince. Users are managed in Settings → **Members**.

A knowledge base is called a **document set** in Margince. Document sets are filed in Settings → **Knowledge**.

The FAQ, the help or the handbook search is called **Ask your documents** in Margince. Open it from the command palette with ⌘K (Ctrl+K).

A to-do list or a task inbox is called the **Worklist** in Margince. It is on **Home**.

An API key or an access token for an AI agent is called a **passport** in Margince. You mint one in Settings → **Agents**.

A writing style or tone profile is called **Voice DNA** in Margince. It is set up in Settings → **Writing voice**.

A theme, dark mode or light mode is called **Appearance** in Margince settings, and **Theme** in the account menu.

A workspace or tenant is your own **company** in Margince: everything a setting marked "Company" changes.

## Where it runs

Margince can run as a hosted service, on your own servers, or entirely on one
machine — including the AI model — for teams whose data cannot leave the
building. The behaviour described in this handbook is the same in all three.

It is licensed under BUSL-1.1, and you get the source.

### Is there a Margince mobile app?
There is no Margince app to install from an app store. Margince runs in the browser on a phone as well as on a computer: on a phone, a bottom bar holds **Home**, **Contacts** and **Deals**, the agent in the middle, and **More** for everything else.
Dragging deal cards on the board does not work on touch screens; use the deal's **Stage** ladder instead.
Also called: iPhone app, Android app, phone app, use Margince on mobile.

## Where to go next

If you are a new user, read [Contacts, companies, leads, deals and
projects](records.md) and [Leads, deals and projects](leads-deals-and-projects.md),
then [The pipeline](the-pipeline.md).

If you are setting the product up, read [Settings](settings.md) and
[Seats, roles and who can see what](seats-roles-and-access.md) first, then
[Capture](capture.md).

If you are responsible for compliance, go straight to
[What is kept, what is destroyed](retention-exports-and-deletion.md).
