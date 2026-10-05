# Settings

Margince settings open from the **account menu** at the top right. They are not
in the sidebar. Which settings pages you see follows your permissions.

### How do I open Settings?
To open Settings in Margince, open the account menu at the top right of the screen and choose **Settings**.
Settings opens on **Overview**, which lists every page you can open. The page list down the side is grouped **You**, **Company**, **People**, **Sales**, **Data**, **AI** and **Governance**.
To go straight to one page, press ⌘K (Ctrl+K) and type its name, or type into **Search settings** at the top of the settings list.
Settings is not in the side list on the left.
Also called: preferences, admin settings, configuration, options.

### What settings pages are there?
Margince Settings groups its pages like this:
- **You**: Account, Meetings, Writing voice, Agents, Notifications, Connections, Capture activity.
- **Company**: Company profile, Sign-in and apps.
- **People**: Members, Teams, Roles and permissions, Seats and license.
- **Sales**: Pipelines, Stage automation, Lead handling, Acquisition sources, Outcome reviews, Responsibility roles, Fields, Tags, Products and offers.
- **Data**: Capture rules, Integrations, Knowledge, Data import.
- **AI**: AI usage, AI models, AI call log, Automations.
- **Governance**: Privacy and retention, Audit log, System health, Extensions, Reset data.
You see only the pages your permissions open. The Reset data page appears only where the deployment enables it.

### Which settings page do I need for my own account?
Your own things in Margince are under **You** in Settings.
- Change your password, display name, email signature, language or theme (appearance): **Account**.
- Set when customers can book you: **Meetings**, in **Bookable hours**.
- Set up how your drafts sound: **Writing voice**.
- Choose how each kind of notification reaches you: **Notifications**.
- Create or revoke an API key for an AI agent: **Agents**.
- Connect your mailbox or calendar, or import from LinkedIn: **Connections**.
- Keep a sender out of capture, or see why an email did not arrive: **Capture activity**.
See [Your own settings](your-own-settings.md) for each one step by step.

### Which settings page do I need for colleagues and the company?
Company-wide things in Margince each have one settings page.
- Invite a colleague, change a user's role, deactivate a user: **Members**.
- Create a team or change who is in it: **Teams**.
- Create, rename, narrow or archive a role: **Roles and permissions**.
- See how many seats are used: **Seats and license**.
- Change the company name, timezone, base currency or exchange rates: **Company profile**.
- Set up single sign-on or the Google and Microsoft apps: **Sign-in and apps**.
- Mark your own email domains, or refuse a domain: **Capture rules**.
- Set up webhooks or a contact-data provider: **Integrations**.

### Which settings page do I need for sales and data?
Sales and data settings in Margince each have one page.
- Change deal stages: **Pipelines**.
- Lead sources, disqualify reasons and the first-response target: **Lead handling**.
- Where a deal came from: **Acquisition sources**.
- Won and lost questions: **Outcome reviews**.
- Add a custom field: **Fields**. Rename a tag everywhere: **Tags**.
- The price list, rate card and offer templates: **Products and offers**.
- Upload a document set to ask questions of: **Knowledge**.
- Import a CSV of companies, contacts or prospects: **Data import**.

### Which settings page do I need for AI and governance?
AI and governance settings in Margince each have one page.
- Which model does which work, and provider keys: **AI models**.
- Trigger-and-action rules: **Automations**.
- The monthly AI allowance and what was spent: **AI usage**. Each call that ran: **AI call log**.
- Consent purposes, retention and privacy requests: **Privacy and retention**.
- Who did what, and when: **Audit log**.
- Background jobs and rebuilding the search index: **System health**.
- Extension units and their grants: **Extensions**.

### Why can't I see a settings page?
A Margince settings page you cannot see is one your role's permissions do not open, or one your deployment has not enabled.
1. Type its name into **Search settings**, or press ⌘K (Ctrl+K). A page you may only read is still listed, under **Read-only settings** on **Overview**.
2. If you open its address and it is not yours, the page says so and keeps the address, so you can send it to an administrator.
Only an administrator can change your role or permissions, in **Members**.
Also called: settings missing, no access to settings, greyed out.

The settings list down the side shows what you can change. A page whose every
control is closed to you is not in it, but you can still open it. Overview
lists everything you can open, in parts: **Your settings**,
**Editable settings** (the side list again) and **Read-only settings**, the
rest. A page you can read and not change says so once, at the top: "Your role
cannot change these settings."

## Whose settings am I changing?

Every Margince settings page carries a badge beside its heading saying who a
change there affects:

| Badge | What it means |
|---|---|
| **Only you** | Your own seat. Nobody else sees the difference. |
| **Company** | Everyone in this company. |
| **Installation** | The whole deployment, such as sign-in, seats and model routing. |
| **Mixed** | The page holds settings of more than one kind. Read the card. |

Three pages are **Mixed**, each for a named reason: *Connections* and *Capture
activity* sit among your own settings but each carries one card that binds
everybody, and *Integrations* holds an installation-wide provider setting beside
company-level wiring.

---

The pages under **You** are on [Your own settings](your-own-settings.md).

# The rest of settings

The rest of Margince settings is six groups beyond **You**. Which pages you see,
and whether you can change them, follows your role. The table at the end sums it up.

## Company profile

The Company profile page in Settings (group **Company**) holds your own
company's name, currency and business context. It is an administrator's page:
Admin and Ops reach it, and a sales seat does not. Sign-in methods and the Google
and Microsoft apps have their own **Sign-in and apps** page in the same group.

**Installation** and **Currency** hold the company's name, timezone and base
currency, which Admin and Ops change.

**Currency rates**: "Exchange rates that convert foreign-currency amounts to
the base currency. New rates take effect today or later; past rates never
change." So a new rate never changes last quarter's figures. Seeing the rates
takes the exchange-rate read permission, which the Admin and Ops roles hold by
default; a custom role can be given it too.

**Company context** is what Margince knows about your own company, where it read
it from, and a place to tell it directly. Only an Admin sees this card and edits
it; Ops finds the page without it.

## Members, Teams, and Seats and license

The Members, Teams, Roles and permissions, and Seats and license pages sit in
the Settings group *People*. One page each: the roster, the team list, the
roles, and the seat count with the licence beside it.

**Members**: "Everyone with a seat, and what each can access." Invite, change
role, deactivate, reactivate. Only administrators see this page. Every seat can
still look a colleague up in the share and assignee pickers on a record.

**Teams**: "Team membership, which decides team-scoped record access." Create one,
archive it, and open a team to add or remove its members. Being in a team grants
no access on its own, with one exception: a **Team lead** reads and works their
team's records, so for them a team is what "their team" means. For every other
shipped role a team matters when a record is shared with it. See
[Seats, roles and who can see what](seats-roles-and-access.md#row-scope-which-records-not-which-kinds).

**Seats and license** shows the installation's entitlement against what is in use. See
[the licence](seats-roles-and-access.md#the-licence).

## Integrations

The Integrations page in Settings (group **Data**) shows what the installation is wired to, as opposed to what one colleague connected.

**Contact data** is a licensed provider of contact data, with its budget and
refresh policy. **Webhooks** are "Outbound subscriptions that receive signed HTTP
POSTs for chosen events"; deliveries can be inspected and replayed.

## Extensions

The Extensions page in Settings (group **Governance**) lists the add-ons your
installation was built with and what each may reach. You cannot add one from
inside the app. Each add-on names what it is for, its version, and the kinds of
record it adds.

If an add-on's screen shows nothing, give a role access to it here with its
switches. The built-in roles start with no access to an add-on's records. An
add-on that adds no records, such as a country-specific rule pack, is listed
with nothing to grant. Admin and Ops both reach the page.

## Capture rules

The Capture rules page in Settings (group **Data**) decides what enters the CRM,
in the order the page puts it:

**Email sharing**, at the top: whether captured mail is shared with colleagues.
It applies to everybody, so it lives here. Your own **Connections** page shows
the current setting and links here.

**Own email domains.** "Domains that belong to this company. Messages between
colleagues are not stored for anyone, including you." Be careful: "Mail skipped
while registered is never offered again by any mailbox."

**Enrichment.** Whether captured companies are enriched automatically.

**Consumer mail domains.** Which domains count as personal mailboxes. "Mail from
a consumer mailbox creates the contact but never a company." Margince ships the
list; you can add a missing domain or override a wrong entry.

**Refused domains.** Which domains this installation refuses a company, and what
decided each one: a model result, a heuristic, or a human. Allowing a domain
back in makes Margince decide again whether it is a company.

## The Sales group

The Sales group in Settings has one page per subject: the fields a record
carries, the stages it moves through, the vocabularies it picks from, and the
priced things that go on an offer.

- **Pipelines**: the stages a deal moves through, for the whole company. See
  [The pipeline](the-pipeline.md#pipelines-and-stages).
- **Stage automation**: "The track record of each stage transition before it
  moves deals automatically." See
  [The pipeline](the-pipeline.md#stage-automation-the-evidence-before-trusting-a-move).
- **Lead handling**: lead sources, disqualify reasons, and the first-response
  target, which is off by default.
- **Acquisition sources**: "Business channels a deal can be attributed to. Lead
  sources, which record how a record entered Margince, are separate."
- **Outcome reviews**: "Questions a rep answers when a deal is won or lost."
  One set for won, one for lost.
- **Responsibility roles**: "What a colleague or team can be accountable for on
  a company, deal or project. A role grants no access to the record." You
  choose what each one applies to and who it can be assigned to when you create
  it. A role is retired through its switch rather than deleted, so an assignment
  that carried it stays readable.
- **Fields**: the custom columns this company keeps beyond the ones every
  installation has. See
  [Contacts, companies, leads, deals and projects](records.md#custom-fields).
- **Tags**: "Shared tags. Renaming a tag here renames it on every record."
- **Products and offers**: what this company sells, and the templates an offer
  starts from.

Every seat can read all of them. All but two are an operator's to change, so for
most seats they appear under **Read-only settings** on **Overview**. Two stay in
a sales seat's side list: **Products and offers**, which a sales seat authors,
and **Outcome reviews**, which a rep opens to read the questions.

## AI usage, AI models, AI call log, and Automations

The AI group in Settings has the pages **AI usage**, **AI models**, **AI call
log** and **Automations**.

**Model routing**, on **AI models**, shows which model serves each kind of work,
listed by activity. What you see is the current setting; **AI call log** shows
what ran. Shared bindings sit under **Advanced**, and changing one can move
several activities at once. Prices show input and output cost per million
tokens. To know where data is processed and what it costs, check the model and
provider shown on each row, not the tier name. Each row in **AI tasks** names its tier, and **View
calls** opens the **AI call log** narrowed to that task.

Changes take effect within about a minute; a call in flight keeps its binding.

**Providers** lists each provider's key, and where it is reached: a host, OpenRouter
hosts, or a Vertex location. A second badge shows when a provider is not
answering. See [AI providers](ai-providers.md).

**Automations** is the trigger-and-action catalogue. Three rules watch a Live
List and, when a record joins or leaves it, add a task, notify the rule's owner,
or add the record to a Shortlist. They run from the 15-minute check, once per
record the owner can see. More than 100 changes in one check, an archived list
or a broken filter pause the rule until its owner resumes it.

**Monthly AI allowance**: Admin and Ops set it; Management can read it and not
change it. The default is 12 million tokens per active full user each month,
pooled across the company. It is a shared company pool measured in tokens, with
no per-person quota and no money cap. You can override the calculation with a fixed company total, which
does not delete the per-user figure underneath. With no eligible users, the
calculation counts one.

The allowance resets at the start of each calendar month, UTC. At **80%**,
work moves to lower tiers; the model can stay the same where two tiers use the
same model. At **100%**, background AI work waits and work you ask for
uses the lowest tier; search indexing keeps running, and still counts.

Raising the allowance lets waiting website reads, account scans and voice builds
run on the next pass, if the provider answers. The waiting counts cover only
those three kinds of work. Work requested by someone whose access was revoked
waits until the access is restored.

Preview an allowance or model change before saving. A save is refused if the
stored configuration has moved since you previewed it, so you never save against
an answer you did not see.

**AI usage** and **AI call log** show "AI spend this month against the
allowance", and "each model request and its response". Anything that failed
outright stays visible in **Background jobs**, on **System health**, instead.

## Knowledge

The Knowledge page in Settings (group **Data**) holds your company's **Document
sets**: "Document collections this company can query. Answers use only filed
documents, and questions they do not cover are refused."

The **Margince handbook** is always one of them; it is filed with every installation and
updated with each release. **Data import** is its own page in the same group.

To ask a document set a question, open the command palette with ⌘K (Ctrl+K) and
choose **Ask your documents**.

See [Documents and files](documents-and-files.md#document-sets--asking-your-documents-questions).

## Privacy and retention

The Privacy and retention page in Settings (group **Governance**) holds consent,
retention and privacy requests. The **Audit log** is its own page beside it, because the two answer to
different permissions.

Together they hold:

- **Consent purposes**: append-only. A purpose cannot be renamed or removed once
  created.
- **Retention**: how long each kind of record is kept, and what happens when its
  window runs out.
- **Restricted records**: what a statutory obligation is holding after an
  erasure.
- **Privacy requests**: data-subject requests with their deadlines.
- **Audit log**: every action, attributed.

Retention and Restricted records are visible to Admin and Ops. The audit log and
privacy requests each need their own permission, and only the Admin role holds
them by default, because they name every actor and whoever asked.

Full detail in
[What is kept, what is destroyed](retention-exports-and-deletion.md).

## System health, Data import, and Reset

System health and Reset data sit in the **Governance** group, and Data import in
**Data**.

- **Import file**, on **Data import**: a CSV of prospects (leads), companies or
  contacts, up to 10 MB unless the deployment sets another limit. Nothing is
  written until you review the preview.
- **Search index**: rebuilding the index behind search and the AI's retrieval.
- **Background jobs**: "Queued background jobs and failed jobs by owner."
  Admin and Ops.
- **AI provider status**: whether each AI provider is answering, as the server
  and the background worker have seen it, merged into one view. See [AI providers](ai-providers.md).
- **Mail capture checks**: whether mail capture's background repair passes keep
  up: when each last succeeded, how many contacts and threads are waiting in
  each mailbox, and how many filed meetings are held back across the whole
  installation. Counts only; the contacts and messages stay visible to their
  mailbox owner alone. Same readers as Background jobs.
- **Reset data**: returns an installation to its first-boot state and deletes
  everything in it. It appears only where whoever runs the installation has
  turned it on.

---

## Which settings you get

Margince settings pages follow permissions. A custom role holding the right
permission reaches the page, and an Admin whose role lost a permission stops
reaching the page that needs it.

**Can I open it?** Search finds it, **Overview** lists it, and its address
works. **Can I change it?** It is in the side list, and its controls are live.

A **read-only seat** keeps every page it could read and loses every control that
writes. Your own language and appearance still work, because they change only
your browser.

| You are | You can change | You can also look up |
|---|---|---|
| A sales seat | Your own pages, Products and offers, Outcome reviews, Capture rules (adding only) | The rest of the Sales group, and Knowledge |
| A team lead | The same | The same |
| Management | The same | The above plus AI usage, model calls, seat counts and the sign-in status, all read-only |
| Ops | Most operational pages, including the model rates | The rest, including the extension list, whose access switches Ops cannot change |
| Admin | Everything the installation has turned on | Nothing further |

A sales seat reaches **Capture rules**, because one card there asks for a
permission every sales role holds. To keep sales seats out, change that
permission on their role.

On that page a sales seat may only add entries, such as a consumer-mail domain
the shipped list missed. It may not change a setting or an entry that is
already there; that is for Admin and Ops.

The **company context** is what Margince's AI knows about your own company.
Only an Admin reads or changes it.

Most pages follow permissions, including the members list, the audit log and
the privacy requests. A custom role with the right permission reaches the page,
and an Admin whose role lost it does not.

A few things ask for the Admin role itself. Two matter to most readers: only an
Admin may act on another Admin's account or hand out the Admin role, and only an
Admin reads or changes the company context. Changing who is on a team and widening a role are the Admin's too
([Seats, roles and who can see what](seats-roles-and-access.md) has the rules),
and some repair and maintenance actions ask for the role. Separately, the last
Admin cannot be demoted or deactivated, so an installation always keeps one.

## Two things administrators should decide early

Two Margince settings are worth deciding before anything else.

**Your own email domains.** Get these right before you connect mailboxes. They decide what counts
as internal, and the decision is not fully reversible.

**Your retention rules.** Your company starts with retention rules already
running. Read them in **Privacy and retention** before your first deletion
happens.
