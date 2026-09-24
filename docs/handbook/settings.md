# Settings

Margince settings open from the **account menu** at the top right, not from the
sidebar. Settings has seven groups and 32 pages, and which of them you see
follows your permissions.

### How do I open Settings?
To open Settings in Margince, open the account menu at the top right of the screen and choose **Settings**.
Settings opens on **Overview**, which lists every page you can open. The page list down the side is grouped **You**, **Company**, **People**, **Sales**, **Data**, **AI** and **Governance**.
To go straight to one page, press ⌘K (Ctrl+K) and type its name, or type into **Search settings** at the top of the settings list.
Settings is not in the side list on the left.
Also called: preferences, admin settings, configuration, options.

### What settings pages are there?
Margince Settings has 32 pages in seven groups:
- **You**: Account, Writing voice, Agents, Connections, Capture activity.
- **Company**: Company profile, Sign-in and apps.
- **People**: Members, Teams, Seats and license.
- **Sales**: Pipelines, Stage automation, Lead handling, Acquisition sources, Outcome reviews, Responsibility roles, Fields, Tags, Products and offers.
- **Data**: Capture rules, Integrations, Knowledge, Data import.
- **AI**: Models and routing, Automations, AI usage, Model calls.
- **Governance**: Privacy and retention, Audit log, System health, Extensions, Reset data.
You see only the pages your permissions open. The Reset data page appears only where the deployment enables it.

### Which settings page do I need for my own account?
Your own things in Margince are under **You** in Settings.
- Change your password, display name, email signature, language or theme (appearance): **Account**.
- Set when customers can book you: **Account**, in **Bookable hours**.
- Set up how your drafts sound: **Writing voice**.
- Create or revoke an API key for an AI agent: **Agents**.
- Connect your mailbox or calendar, or import from LinkedIn: **Connections**.
- Keep a sender out of capture, or see why an email did not arrive: **Capture activity**.
See [Your own settings](your-own-settings.md) for each one step by step.

### Which settings page do I need for colleagues and the company?
Company-wide things in Margince each have one settings page.
- Invite a colleague, change a user's role, deactivate a user: **Members**.
- Create a team or change who is in it: **Teams**.
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
- Which model does which work, and provider keys: **Models and routing**.
- Trigger-and-action rules: **Automations**.
- The monthly AI allowance and what was spent: **AI usage**. Each call that ran: **Model calls**.
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

**The settings list down the side shows what you can change.** A page whose
every control is closed to you is not in it — prominence, not permission.
**Overview lists everything you can open**, in parts: **Your settings**,
**Editable settings** — the side list again — and **Read-only settings**, the
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

Your own five pages — Account, Writing voice, Agents, Connections and Capture
activity — are on [Your own settings](your-own-settings.md).

# The rest of settings

The rest of Margince settings is six groups beyond **You**. Which pages you see,
and whether you can change them, follows your role — see the table at the end.

## Company profile

The Company profile page in Settings (group **Company**) holds your own
company's name, currency and business context. Sign-in methods and the Google
and Microsoft apps have their own **Sign-in and apps** page in the same group.

**Installation** and **Currency** — the company's name, timezone and base
currency. **Currency rates** — "Exchange rates that convert foreign-currency amounts to
the base currency. New rates take effect today or later; past rates never
change." That last clause is the point: setting a rate today cannot rewrite what
last quarter reported. Only an administrator or an operations user can see the
rates.

**Company context** — what Margince knows about your own company, where it read
it from, and a place to tell it directly.

## Members, Teams, and Seats and license

The Members, Teams and Seats and license pages sit in the Settings group
*People* — the one place in the product where that word still means the
humans who work here rather than the record type. One page each: the roster,
the team list, and the seat count with the licence beside it. **Roles and
permissions** is not built yet, and is deliberately absent from the navigation
rather than present and empty.

**Members** — "Everyone with a seat, and what each can access." Invite,
change role, deactivate, reactivate. The settings *entry* is administrators',
because it follows those verbs rather than the roster underneath — every seat
can still look a colleague up, since the share and assignee pickers read the
same roster beside the record where you need it.

**Teams** — "Team membership, which decides team-scoped record access." Create one,
archive it, and open a team to add or remove its members. Being in a team grants
no access on its own — with one exception: a **Team lead** reads and works their
team's records, so for them a team is what "their team" means. For every other
shipped role a team matters when a record is shared with it. See
[Seats, roles and who can see what](seats-roles-and-access.md#row-scope-which-records-not-which-kinds).

**Seats and license** — the installation's entitlement against what is in use. See
[the licence](seats-roles-and-access.md#the-licence).

## Integrations

The Integrations page in Settings (group **Data**) shows what the installation is wired to, as opposed to what one colleague connected.

**Contact data** is a licensed provider of contact data, with its budget and
refresh policy. **Webhooks** are "Outbound subscriptions that receive signed HTTP
POSTs for chosen events"; deliveries can be inspected and replayed.

## Extensions

The Extensions page in Settings (group **Governance**) lists every extension unit this installation was built with, and what each may reach.

A unit is software composed in at build time, not installed from inside the app,
so this page reports what is there rather than offering anything to add. Each
unit names what it is for, the version it declares, and the permission objects
it registered.

Those permission objects are why the page exists. A unit that owns records gates
them on names no seeded role has heard of, so until somebody grants a role read
on them, every seat opens the unit's screen and sees nothing. The switches here
are that grant. A unit that registers none — a jurisdiction pack, say — is
listed with nothing to grant, which is the correct and common answer. Admin and
Ops both reach it.

## Capture rules

The Capture rules page in Settings (group **Data**) holds the posture that decides what enters the CRM at all, in the order
the page puts it:

**Email sharing**, at the top — whether captured mail is shared with colleagues.
It is everybody's business rather than one seat's, which is why it lives here;
your own connections page states what it currently says and links here.

**Own email domains.** "Domains that belong to this company. Messages between
colleagues are not stored for anyone, including you." Be careful: "Mail skipped
while registered is never offered again by any mailbox."

**Enrichment.** Whether captured companies are enriched automatically.

**Consumer mail domains.** Which domains count as personal mailboxes. "Mail from
a consumer mailbox creates the contact but never a company." Margince ships the
list; you can add a missing domain or override a wrong entry.

**Refused domains.** Which domains this installation refuses a company, and what
decided each one — a model result, a heuristic, or a human. Allowing a domain
back in reopens the company question rather than merely clearing a flag.

## The Sales group

The Sales group in Settings has nine pages, one per subject: the fields a record
carries, the stages it moves through, the vocabularies it picks from, and the
priced things that go on an offer.

- **Pipelines** — the stages a deal moves through, for the whole company. See
  [The pipeline](the-pipeline.md#pipelines-and-stages).
- **Stage automation** — "The track record of each stage transition before it
  moves deals automatically." See
  [The pipeline](the-pipeline.md#stage-automation-the-evidence-before-trusting-a-move).
- **Lead handling** — lead sources, disqualify reasons, and the first-response
  target, which is off by default.
- **Acquisition sources** — "Business channels a deal can be attributed to. Lead
  sources, which record how a record entered Margince, are separate."
- **Outcome reviews** — "Questions a rep answers when a deal is won or lost."
  One set for won, one for lost.
- **Responsibility roles** — "What a colleague or team can be accountable for on
  a company, deal or project. **A role grants no access to the record.**" You
  choose what each one applies to and who it can be assigned to when you create
  it. A role is retired through its switch rather than deleted, so an assignment
  that carried it stays readable.
- **Fields** — the custom columns this company keeps beyond the ones every
  installation has. See
  [Contacts, companies, leads, deals and projects](records.md#custom-fields).
- **Tags** — "Shared tags. Renaming a tag here renames it on every record."
- **Products and offers** — what this company sells, and the templates an offer
  starts from.

Every seat can READ all nine. Seven are an operator's to change, so for most
seats they appear under **Read-only settings** on **Overview**. Two stay in a sales seat's side list: **Products and offers**, which a sales seat
authors, and **Outcome reviews**, where reading the questions is itself the
point of opening the page.

## Models and routing, Automations, AI usage, and Model calls

The AI group in Settings has four pages: **Models and routing**,
**Automations**, **AI usage** and **Model calls**.

**Model routing** — which model serves each kind of work, presented by activity
rather than by tier. What you see is the current policy; **Model calls** is what
actually ran. Shared bindings sit under Advanced, and changing one can move
several activities at once. Prices name input and output cost per million tokens
separately, rather than one unexplained arrow, and a tier name proves nothing
about where data is processed or what it costs — read the binding.

Changes take effect without a restart: a running process picks up a saved
binding within about a minute, and a call already in flight keeps the one it
started with.

**Provider keys** — your own keys for whichever provider you use. Margince can
also run entirely against a local model with no cloud key at all.

**Automations** — the trigger-and-action catalogue. **Monthly AI allowance** — Admin and Ops set it; Management can read it and not
change it. The default is **12 million tokens per active full user each month**,
pooled across the company. It is not an individual quota and not a spending cap
in money. You can override the calculation with a fixed company total, which
does not delete the per-user figure underneath. With no eligible users, the
calculation counts one.

The allowance resets at the start of each calendar month, UTC. **At 80%**,
routing moves to lower tiers; the actual model can stay the same where two tiers
share one binding. **At 100%**, background completions wait and interactive
calls use the lowest tier; search embeddings keep running, and still count.

Raising the allowance does not just unblock the future: saved website reads,
account scans and voice builds that were waiting become runnable on the next
pass, each carrying its original request, authority and attempt limits — which
is not a promise a provider will answer. The waiting counts cover exactly those
saved carriers, not every AI task. A requester whose access was revoked stays
parked until it is restored.

Preview an allowance or model change before saving. A save is refused if the
stored configuration has moved since you previewed it, so you never save against
an answer you did not see.

**AI usage** and **Model calls** — "AI spend this month against the
allowance", and "each model request and its response". Anything that failed
outright stays visible in **Background jobs**, on **System health**, instead.

## Knowledge

The Knowledge page in Settings (group **Data**) holds your company's **Document
sets**: "Document collections this company can query. Answers use only filed
documents, and questions they do not cover are refused." The **Margince
handbook** is always one of them; it is filed with every installation and
updated with each release. **Data import** is its own page in the same group.

To ask a document set a question, open the command palette with ⌘K (Ctrl+K) and
choose **Ask your documents**.

See [Documents and files](documents-and-files.md#document-sets--asking-your-documents-questions).

## Privacy and retention

The Privacy and retention page in Settings (group **Governance**) holds consent,
retention and privacy requests. The **Audit log** is its own page beside it: the two
answer to different permissions, so a reader could hold one and not the other.

Five things, and they are the heart of the compliance story:

- **Consent purposes** — append-only. A purpose cannot be renamed or removed once
  created.
- **Retention** — how long each kind of record is kept, and what happens when its
  window runs out.
- **Restricted records** — what a statutory obligation is holding after an
  erasure.
- **Privacy requests** — data-subject requests with their deadlines.
- **Audit log** — every action, attributed.

Retention and Restricted records are visible to Admin and Ops. The audit log and
the privacy requests are **Admin only**, because they name every actor and
whoever asked.

Full detail in
[What is kept, what is destroyed](retention-exports-and-deletion.md).

## System health, Data import, and Reset

System health, Data import and Reset data are three Settings pages (groups
**Governance** and **Data**), split so that a reindex, a queue
reading and emptying the installation are no longer three buttons on one screen.

- **Import file**, on **Data import** — a CSV of prospects (leads), companies or
  contacts, up to 10 MB unless the deployment sets another limit. Nothing is
  written until you review the preview.
- **Search index** — rebuilding the index behind search and the AI's retrieval.
- **Background jobs** — "Queued background jobs and failed jobs by owner."
  Admin and Ops.
- **Reset data** — returns an installation to its first-boot state. It only
  appears where the installation has deliberately armed the capability. Treat it
  as what it is.

---

## Which settings you get

Margince settings pages follow **permissions**, not role names. A custom role holding the right
permission reaches the page with no change to the product, and an Admin whose
role lost a permission stops reaching the page that needs it.

**Can I open it?** Search finds it, **Overview** lists it, and its address
works. **Can I change it?** It is in the side list, and its controls are live.

A **read-only seat** is the clearest case of the two coming apart: it keeps every
page it could read and loses every control that writes. Your own preferences
still work — language and appearance are about your browser, not anyone's
records.

| You are | You can change | You can also look up |
|---|---|---|
| A sales seat | Your own five pages, Products and offers, Outcome reviews, Company profile, Capture rules (adding only) | The rest of the Sales group, and Knowledge |
| A team lead | The same | The same |
| Management | The same | The above plus AI usage, model calls, seat counts and the sign-in status — all of them readable and none of them theirs to change |
| Ops | Most of the operational catalog, including the model rates | The rest, the extension inventory among them — Ops reads it and cannot change the grants |
| Admin | Everything the deployment has armed | — |

That first row surprises readers, so it is worth saying plainly: a sales seat
reaches **Capture rules** and the **company profile**, because those cards ask
for a permission every sales role holds. If that is not what you want, the fix is
the permission, not the page.

What a sales seat can do there is narrower than "edit", and the difference
matters: it may **add** — a consumer-mail domain the shipped list missed, for
instance — and it may not change a setting or touch an entry that is already
there. Changing what exists is Admin's and Ops's.

On an installation without the company-context capability, a sales seat has no
writable card on the company profile.

Nearly everything is a permission now, including the three that used to be role
checks: administering members answers to `user_admin`, the audit log to
`audit_log`, and the privacy queue to `privacy_request`. A custom role granted
one of those reaches the page, and an Admin whose role lost it does not.

Three things still ask for the literal Admin role rather than a permission, and
one of them is not about recovery: **only an Admin may act on another Admin's
account**, or hand out the Admin role. The other two are the last-admin rule and
deployment-level resets.

## Two things administrators should decide early

Two Margince settings are worth deciding before anything else. **Your own email
domains.** Get these right before you connect mailboxes. They decide what counts
as internal, and the decision is not fully reversible.

**Your retention posture.** Your company already has six retention rules
running. Read them before your first deletion happens rather than after.
