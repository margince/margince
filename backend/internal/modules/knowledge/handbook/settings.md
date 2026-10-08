<!-- prose:plain -->
# Settings

Margince settings open from the **account menu** at the top right. They are not in the sidebar. Which settings pages you see follows your permissions.

### How do I open Settings?
To open Settings in Margince, open the account menu at the top right of the screen and choose **Settings**.
Settings opens on **Overview**, which lists every page you can open. The page list down the side is in groups: **You**, **Company**, **People**, **Sales**, **Data**, **AI** and **Governance**.
To go right to one page, press ⌘K (Ctrl+K) and type its name. Or type into **Search settings** at the top of the settings list.

Settings is not in the side list on the left.
Also called: preferences, admin settings, configuration, options.

### What settings pages are there?
Margince Settings puts its pages in these groups:
- **You**: Account, Meetings, Writing voice, Agents, Notifications, Connections, Capture activity.
- **Company**: Company profile, Sign-in and apps.
- **People**: Members, Teams, Roles and permissions, Seats and license.
- **Sales**: Pipelines, Stage automation, Lead handling, Acquisition sources, Outcome reviews, Responsibility roles, Fields, Tags, Products and offers.
- **Data**: Capture rules, Integrations, Knowledge, Data import.
- **AI**: AI usage, AI models, AI call log, Automations.
- **Governance**: Privacy and retention, Audit log, System health, Extensions, Reset data.

You see only the pages your permissions open. The Reset data page appears only where the installation turns it on.

### Which settings page do I need for my own account?
Your own things in Margince are under **You** in Settings.
- Change your password, display name, email signature, language or theme (appearance): **Account**.
- Set when customers can book you: **Meetings**, in **Bookable hours**.
- Set up how your drafts sound: **Writing voice**.
- Choose how each kind of notification reaches you: **Notifications**.
- Create or take back an API key for an AI agent: **Agents**.
- Connect your mailbox or calendar, or import from LinkedIn: **Connections**.
- Keep a sender out of capture, or see why an email did not come in: **Capture activity**.

See [Your own settings](your-own-settings.md) for each one step by step.

### Which settings page do I need for colleagues and the company?
Things for the whole company in Margince each have one settings page.
- Ask a colleague in, change a user's role, turn a user off: **Members**.
- Create a team or change who is in it: **Teams**.
- Create, rename, narrow or archive a role: **Roles and permissions**.
- See how many seats are in use: **Seats and license**.
- Change the company name, time zone, base currency, date format, time format or exchange rates: **Company profile**.
- Set up single sign-on or the Google and Microsoft apps: **Sign-in and apps**.
- Mark your own email domains, or refuse a domain: **Capture rules**.
- Set up webhooks or a provider of contact data: **Integrations**.

### Which settings page do I need for sales and data?
Sales and data settings in Margince each have one page.
- Change deal stages: **Pipelines**.
- Lead sources, reasons to drop a lead, and the first-reply target: **Lead handling**.
- Where a deal came from: **Acquisition sources**.
- Won and lost questions: **Outcome reviews**.
- Add a custom field: **Fields**. Rename a tag on every record: **Tags**.
- The price list, rate card and offer templates: **Products and offers**.
- Upload a document set to ask questions of: **Knowledge**.
- Import a CSV of companies, contacts or prospects: **Data import**.

### Which settings page do I need for AI and governance?
AI and governance settings in Margince each have one page.
- Which model does which work, and provider keys: **AI models**.
- Rules that do an action when something happens: **Automations**.
- The monthly AI allowance and what was used: **AI usage**. Each call that ran: **AI call log**.
- Consent purposes, how long records are kept, and privacy requests: **Privacy and retention**.
- Who did what, and when: **Audit log**.
- Jobs that run in the background, and building the search index again: **System health**.
- Add-ons and what they may reach: **Extensions**.

### Why can't I see a settings page?
A Margince settings page you cannot see is one your role's permissions do not open, or one your installation has not turned on.
1. Type its name into **Search settings**, or press ⌘K (Ctrl+K). A page you may only read is still listed, under **Read-only settings** on **Overview**.
2. If you open its address and it is not yours, the page says so and keeps the address. You can send that address to an administrator.

Only an administrator can change your role or permissions, in **Members**.
Also called: settings missing, no access to settings, greyed out.

The settings list down the side shows what you can change. A page where every control is closed to you is not in it, but you can still open it. Overview lists everything you can open, in parts: **Your settings**, **Editable settings** (the side list again) and **Read-only settings**, the rest. A page you can read and not change says so once, at the top: "Your role cannot change these settings."

## Whose settings are you changing?

Every Margince settings page carries a badge beside its heading. It says who a change there touches:

| Badge | What it means |
|---|---|
| **Only you** | Your own seat. Nobody else sees any change. |
| **Company** | Everyone in this company. |
| **Installation** | The whole installation, such as sign-in, seats and which model does which work. |
| **Mixed** | The page holds settings of more than one kind. Read the card. |

Three pages are **Mixed**, each for a clear reason. *Connections* and *Capture activity* sit with your own settings, but each carries one card that holds for everybody. *Integrations* holds a provider setting for the whole installation beside settings for the company.

---

The pages under **You** are on [Your own settings](your-own-settings.md).

# The rest of settings

The rest of Margince settings is six groups after **You**. Which pages you see, and whether you can change them, follows your role. The table at the end sums it up.

## Company profile

The Company profile page in Settings (group **Company**) holds your own company's name, currency and business facts. It is an administrator's page: Admin and Ops reach it, and a sales seat does not. Sign-in and the Google and Microsoft apps have their own **Sign-in and apps** page in the same group.

**Installation** and **Currency** hold the company's name, time zone and base currency, which Admin and Ops change.

**Date format** and **Time format** sit on the **Installation** card too, and set the notation for formatted dates and times across the interface. For dates the choices are **Use interface language**, **DD.MM.YYYY · 23.09.2026**, **MM/DD/YYYY · 09/23/2026** and **YYYY-MM-DD · 2026-09-23**. For times they are **Use interface language**, **24-hour · 17:30** and **12-hour · 05:30 pm**. Native date fields use the browser's own date picker and date format. Stored dates and time zones stay the same.

**Currency rates** are the exchange rates that turn money in another currency into the base currency. A new rate starts today or later, and a past rate never changes. So a new rate never changes the numbers of past months. Seeing the rates needs the permission to read exchange rates. The Admin and Ops roles hold it by default, and a custom role can get it too.

**Company context** is what Margince knows about your own company, where it read it, and a place to tell it facts yourself. Only an Admin sees this card and edits it; Ops finds the page without it.

## Members, Teams, and Seats and license

The Members, Teams, Roles and permissions, and Seats and license pages sit in the Settings group *People*. There is one page each: the list of members, the list of teams, the roles, and the seat count with the license beside it.

**Members** lists everyone with a seat, and what each one can open. Here you ask colleagues in, change a role, turn a user off and turn them on again. Only a role allowed to see users sees this page: Admin by default, or a custom role given that permission. Every seat can still look a colleague up in the lists for sharing and for picking an owner on a record.

**Teams** holds who is in each team, which decides who can open records shared with a team. Create one, archive it, and open a team to add or remove its members. Being in a team gives no access on its own, with one exception. A **Team lead** reads and works their team's records, so for them a team is what "their team" means. For every other built-in role, a team counts when a record is shared with it. See [Seats, roles and who can see what](seats-roles-and-access.md#row-scope-which-records-not-which-kinds).

**Seats and license** shows how many seats the installation may use against how many are in use. See [the licence](seats-roles-and-access.md#the-licence).

## Integrations

The Integrations page in Settings (group **Data**) shows what the installation is connected to, as against what one colleague connected.

**Contact data** is a licensed provider of contact data, with its budget and a policy for how often it refreshes the data. **Webhooks** send a signed message to another system when events you picked happen. You can look at each one sent and send it again.

## Extensions

The Extensions page in Settings (group **Governance**) lists the add-ons your installation was built with, and what each may reach. You cannot add one from inside the app. Each add-on names what it is for, its release, and the kinds of record it adds.

If an add-on's screen shows nothing, give a role access to it here with its switches. The built-in roles start with no access to an add-on's records. An add-on that adds no records, such as a set of rules for one place, is listed with nothing to give. Admin and Ops both reach the page.

## Capture rules

The Capture rules page in Settings (group **Data**) decides what comes into the CRM, in the order the page shows:

**Email sharing**, at the top: whether captured mail is shared with colleagues. It holds for everybody, so it lives here. Your own **Connections** page shows the current setting and links here.

**Own email domains** are the domains that belong to this company. Mail between colleagues is not stored for anyone, including you. Take note: no mailbox offers again the mail skipped while a domain is on this list.

**Enrichment.** Whether Margince fills in facts on captured companies by itself.

**Consumer mail domains.** Which domains count as private mailboxes. Mail from one of them makes the contact, but never a company. Margince comes with the list; you can add a missing domain or change a wrong entry.

**Refused domains.** Which domains this installation will not make a company for, and what decided each one: a model result, an automatic rule, or a human. Letting a domain back in makes Margince decide again whether it is a company.

## The Sales group

The Sales group in Settings has one page per subject. There are the fields a record carries, the stages it moves through, and the lists of choices it picks from. And there are the priced things that go on an offer.

- **Pipelines**: the stages a deal moves through, for the whole company. See
  [The pipeline](the-pipeline.md#pipelines-and-stages).
- **Stage automation**: the record of how each stage move has gone, before
  Margince moves deals by itself. See
  [The pipeline](the-pipeline.md#stage-automation-the-evidence-before-trusting-a-move).
- **Lead handling**: lead sources, reasons to drop a lead, and the first-reply
  target, which is off by default.
- **Acquisition sources**: the business channels a deal can come from. Lead
  sources, which record how a record came into Margince, are separate.
- **Outcome reviews**: questions a seller answers when a deal is won or lost.
  There is one set for won and one for lost.
- **Responsibility roles**: what a colleague or team can answer for on a
  company, deal or project. A role gives no access to the record. When you
  create one, you choose what it applies to and who can hold it. You turn a role
  off with its switch instead of deleting it, so a record that carried it still
  reads well.
- **Fields**: the custom columns this company keeps on top of the ones every
  installation has. See
  [Contacts, companies, leads, deals and projects](records.md#custom-fields).
- **Tags**: tags everyone shares. Renaming a tag here renames it on every
  record.
- **Products and offers**: what this company sells, and the templates an offer
  starts from.

Every seat can read all of them. All but two are for Admin and Ops to change, so for most seats they appear under **Read-only settings** on **Overview**. Two stay in a sales seat's side list. A sales seat writes **Products and offers**, and a seller opens **Outcome reviews** to read the questions.

## AI usage, AI models, AI call log, and Automations

The AI group in Settings has the pages **AI usage**, **AI models**, **AI call log** and **Automations**.

**Model routing**, on **AI models**, shows which model does each kind of work, listed by activity. What you see is the current setting; **AI call log** shows what ran. Settings that several kinds of work share sit under **Advanced**, and changing one can move several kinds of work at once. Prices show the cost of the text sent in and the text that comes back, per million tokens. To know where data goes and what it costs, check the model and provider shown on each row, not the tier name.

Each row in **AI tasks** names its tier. Its name opens its state, what it does when the provider is down, and **View calls**. **Edit** sets how hard it reasons, and its time limits. Search and retrieval has neither: set its model under **Model tiers**.

Changes start to work in about 60 seconds; a call already running keeps its old setting.

**Providers** lists each provider's key, and where Margince reaches it: a host, OpenRouter hosts, or a Vertex place. A second badge shows when a provider is not answering. See [AI providers](ai-providers.md).

**Automations** is the list of rules that do an action when something happens. Three rules look at a Live List. When a record comes into it or leaves it, they add a task, tell the rule's owner, or add the record to a Shortlist. They run from the check every 15 minutes, once per record the owner can see. More than 100 changes in one check, an archived list or a filter that no longer works stop the rule until its owner starts it again.

**Monthly AI allowance**: Admin and Ops set it; Management can read it but not change it. The default is 12 million tokens each month for each full user who is active, shared by the whole company. It is one company total measured in tokens, with no limit per user and no money limit. You can set a fixed company total instead of the sum, which does not delete the number per user under it. With no users that count, the sum counts one.

The allowance starts again at the start of each calendar month, UTC. At **80%**, work moves to lower tiers; the model can stay the same where two tiers use the same model. At **100%**, AI work in the background waits, and work you ask for uses the lowest tier. Search indexing keeps running, and still counts.

When someone sets a higher allowance, three kinds of waiting work run on the next pass, if the provider answers. They are reads of a company's site, company checks and voice builds. The waiting counts cover only those three kinds of work. Work asked for by someone who lost their access waits until that access comes back.

Look at a preview of an allowance or model change before you save it. Margince refuses a save if the stored setting has changed after your preview, so you never save against an answer you did not see.

**AI usage** and **AI call log** show "AI spend this month against the allowance", and "each model request and its response". Anything that failed for good stays in view in **Background jobs**, on **System health**, instead.

## Knowledge

The Knowledge page in Settings (group **Data**) holds your company's **Document sets**: groups of documents this company can ask questions of. Answers use only filed documents, and Margince refuses questions they do not cover.

The **Margince handbook** is always one of them. It is filed with every installation and updated with each release. **Data import** is its own page in the same group.

To ask a document set a question, open the command palette with ⌘K (Ctrl+K) and choose **Ask your documents**.

See [Documents and files](documents-and-files.md#document-sets--asking-your-documents-questions).

## Privacy and retention

The Privacy and retention page in Settings (group **Governance**) holds consent, how long records are kept, and privacy requests. The **Audit log** is its own page beside it, because the two need different permissions.

Between them they hold:

- **Consent purposes**: you can only add. Once you create a purpose, you cannot
  rename or remove it.
- **Retention**: how long each kind of record is kept, and what happens when its
  time runs out.
- **Restricted records**: what the law makes Margince hold after an erasure.
- **Privacy requests**: requests from a contact about their own data, with their
  due dates.
- **Audit log**: every action, and who did it.

Admin and Ops can see Retention and Restricted records. The audit log and privacy requests each need their own permission. Only the Admin role holds them by default, because they name everyone who acted and everyone who asked.

All the detail is in [What is kept, what is destroyed](retention-exports-and-deletion.md).

## System health, Data import, and Reset

System health and Reset data sit in the **Governance** group, and Data import in **Data**.

- **Import file**, on **Data import**: a CSV of prospects (leads), companies or
  contacts, up to 10 MB unless the installation sets another limit. Nothing is
  written until you check the preview.
- **Search index**: building the index again, behind search and behind what the
  AI finds.
- **Background jobs**: jobs that wait to run, and jobs that failed, by owner.
  Admin and Ops.
- **AI provider status**: whether each AI provider is answering, as the app
  and its background jobs found it, in one view. See [AI providers](ai-providers.md).
- **Mail capture checks**: whether the passes that fix mail capture in the
  background keep up. It shows when each last worked, and how many contacts and
  threads wait in each mailbox. It also shows how many filed meetings are held
  back in the whole installation. It shows counts only; the contacts and messages stay
  in view to their mailbox owner alone. The same readers as Background jobs see
  it.
- **Reset data**: takes an installation back to how it was on its first start.
  It deletes every record and setting, but keeps the company and its users, so
  everyone can still sign in. It appears only where whoever runs the
  installation has turned it on.

## Data import

The Data import page in Settings (group **Data**) brings in a CSV of prospects, companies or contacts. Only an admin or ops user can import. Starting an import and undoing one are in [What is kept, what is destroyed](retention-exports-and-deletion.md).

### What does the import preview tell me?
The Margince **Import preview** counts what the import will do with each row: **Create**, **Update**, **Unchanged** and **Skipped**. The four add up to the rows in the file, and "{rows} rows read, identified by {column}." names the column that tells rows apart. Rows that cannot be imported are listed with their line number. Fix them in the file and choose **Preview import** again. Nothing is written until you choose **Import {rows} rows**.
Also called: test run, check an import, import report.

### What happens if I import the same file again?
Importing the same file again in Margince updates the records that file made the first time, and does not copy them. The screen names the column that tells rows apart: the company name for companies, the email for contacts and prospects. A row whose values did not change counts as **Unchanged**, so a file you already imported reports no work.
Also called: re-import, upload again, update from a spreadsheet.

### What happens to a company I already have?
A company that Margince already holds, and that no import made, is not matched by its name. The import creates a second company and files the two for review as a possible duplicate. To change companies you already have, correct them by their ID instead. A company you cannot see is not counted or named in the preview.
Also called: duplicates on import, import made copies.

### How do I correct companies with a spreadsheet?
To correct companies in Margince from a CSV, give each row the company's ID and map that column to **id**.
1. Use **Export CSV** from **Filters and views**; its **id** column holds each ID.
2. Edit the file, then choose **Start import** in **Settings → Data import**, with **Row type** **Companies**.
3. In **Column mapping**, map **id** to **id**, and the name column to **display_name**.
4. Choose **Preview import**.
An empty ID creates a company. An ID that matches no company is skipped and named.
Also called: bulk edit companies, update companies from Excel, fix company data.

Undo never puts back the old values of a company an import corrected. It only archives the companies that import created.

### What if an import stops partway?
When a Margince import stops partway, the result says **Import stopped partway** and how many rows it read. Choose **Resume import** to go on from that row, not from the start. **Tag for this import** puts every record the import creates under one tag, so you can find them later. Records it only updates keep their own tags.
Also called: import failed, import stopped, continue an import.

---

## Which settings you get

Margince settings pages follow permissions. A custom role with the right permission reaches the page, and an Admin whose role lost a permission stops reaching the page that needs it.

**Can I open it?** Search finds it, **Overview** lists it, and its address works. **Can I change it?** It is in the side list, and its controls work.

A **read-only seat** keeps every page it could read, and loses every control that writes. Your own language and appearance still work, because they change only what your own screen shows.

| You are | You can change | You can also look up |
|---|---|---|
| A sales seat | Your own pages, Products and offers, Outcome reviews, Capture rules (adding only) | The rest of the Sales group, and Knowledge |
| A team lead | The same | The same |
| Management | The same | The above, and also AI usage, model calls, seat counts and the sign-in status, all read-only |
| Ops | Most pages that run the business, the model rates too | The rest, the list of add-ons too, whose access switches Ops cannot change |
| Admin | Everything the installation has turned on | Nothing more |

A sales seat reaches **Capture rules**, because one card there asks for a permission every sales role holds. To keep sales seats out, change that permission on their role.

On that page a sales seat may only add entries, such as a consumer mail domain missing from the list Margince came with. It may not change a setting or an entry that is already there; that is for Admin and Ops.

The **company context** is what the Margince AI knows about your own company. Only an Admin reads or changes it.

Most pages follow permissions, such as the members list, the audit log and the privacy requests. A custom role with the right permission reaches the page, and an Admin whose role lost it does not.

A few things ask for the Admin role itself. Two are for most readers to know. Only an Admin may act on another Admin's account or give out the Admin role. And only an Admin reads or changes the company context.

Changing who is on a team, and giving a role more, are for the Admin too. The rules are in [Seats, roles and who can see what](seats-roles-and-access.md). Some actions that fix things ask for the role too. Apart from that, nobody can move the last Admin to a lower role or turn them off, so an installation always keeps one.

## Two things administrators should decide early

Two Margince settings are worth deciding before anything else.

**Your own email domains.** Get these right before you connect mailboxes. They decide what counts as internal, and you cannot undo all of that decision.

**Your retention rules.** Your company starts with retention rules already running. Read them in **Privacy and retention** before Margince first deletes anything.
