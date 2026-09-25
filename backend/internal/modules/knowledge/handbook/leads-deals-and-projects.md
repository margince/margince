# Leads, deals and projects

A lead is someone you might sell to, a deal is one opportunity with one
company, and a project is the body of work a deal belongs to. This page is how
you create, qualify, disqualify and reopen leads, create deals and projects,
and what each connection between them means. Contacts and companies, and
archiving, undo, custom fields and tags for every record, are in
[Contacts, companies, leads, deals and projects](records.md).

## Leads

> "Leads are kept apart from Contacts. A lead becomes a contact only when you
> qualify it."

A lead needs only a source. Email and LinkedIn address are the fields used to
spot duplicates; a second live lead with the same email is refused.

### How do I create a lead?
To create a lead in Margince, open **Leads** in the sidebar and choose **New lead**.
1. Enter the **Full name** (required).
2. Optionally fill **Email**, **LinkedIn URL**, **Title** and **Company** (free text, not a link to a company record).
3. Pick the **Owner** (required; **Unassigned** is one of the choices).
4. Pick the **Source** (required; it starts on the first source in the list, normally "Created manually").
5. Fill any custom fields, then choose **Create**.
Many leads at once come in through Settings → **Data import**.
Also called: add a prospect, new lead.

### The ladder

A lead has five statuses. The first three are open, the last two are terminal.

| What the screen says | What it means |
|---|---|
| **New** | Nothing has happened yet |
| **Contacted** | We reached out |
| **Engaged** | They answered, or a meeting is booked or held |
| **Qualified** | A contact now exists for this lead |
| **Disqualified** | Closed, with a reason |

Contacted and engaged are set automatically from captured activity, and can also
be set by hand. The record tells you which: "set automatically from captured
activity" or "set by hand".

You may move an open lead to any open step, forwards or back, on its **Lead
status** ladder. The system only ever moves it forwards.

### How do I qualify a lead or convert it into a deal?
To qualify a lead in Margince, open the lead and choose **Qualify**; tick **Also open a deal** to create a deal in the same step.
1. Check the **Contact** preview: "Qualifying creates a new contact." or "Qualifying merges into the existing contact".
2. For a deal, tick **Also open a deal** and set **Pipeline**, **Stage**, **Deal name** and optionally **Amount**.
3. Read **Why** and add an **Evidence note (optional)**.
4. Choose **Qualify**, or **Qualify and open deal**.
**Qualify** is disabled until the lead has an email address and an open status.
Also called: convert a lead, promote a lead, turn a lead into a contact or opportunity.

Qualifying a lead records what triggered it — an inbound reply, a meeting
booked, a meeting held, or your own judgement. The dialog's **Why** line is
filled from the captured activity when there is one, and otherwise reads
"Reason: qualified by you." **Cold outbound with no reply is refused.**
Qualification is engagement, not import and not an outbound touch.

Then one of two things happens, in a single step:

- If the lead's email matches an existing contact, it **merges** into that
  contact. No duplicate is created.
- Otherwise a new contact is created.

Either way the lead's history and activities come with it — nothing is orphaned —
and the lead is marked qualified and archived. You can preview which of the two
will happen before you confirm.

One caution on that preview: if the matching contact is one you are not allowed
to see, the preview says "Qualifying merges into an existing contact you cannot
see." and does not show you the contact. **An absent contact never means "no
match".**

A deal opened in the same step takes the lead's owner and is left with no
company. The amount is in whole units of your installation's base currency.

### How do I reverse a qualification?
To reverse a lead qualification in Margince, open the qualified lead and choose **Reverse qualification**.
1. Enter a **Reason (recorded in the audit trail)**. Without one the dialog says "Enter a reason first."
2. Choose **Reverse**.
The lead goes back to the open status Engaged. A contact the qualification created is archived; a contact it merged into is left unchanged.
It is refused while the contact is a stakeholder on a live deal.
Also called: undo a qualification, unqualify, demote a lead.

What a reversal does depends on how the lead was qualified:

- If it **created** a contact: the contact is archived and the lead comes back to
  the queue at Engaged. Activities captured since the qualification stay on the
  contact's timeline.
- If it **merged** into an existing contact: that contact is untouched. Only the
  lineage pointers are cleared. A field-level un-merge is never attempted,
  because it is lossy and ambiguous.
- **If the contact is a stakeholder on a live deal, it cannot be reversed at
  all.** The product blocks rather than orphaning the deal. Note the test is
  stakeholder, not owner.
- If other records have come to depend on the contact, a reversal that would
  have archived it quietly narrows to clearing the lineage pointers instead.

### How do I disqualify a lead?
To disqualify a lead in Margince, open the lead, choose **More actions**, then **Disqualify**.
1. In **Disqualify {name}?**, pick a **Reason** (required; without one it says "Choose a reason first.").
2. Optionally add a **Note (optional)**.
3. Choose **Disqualify**.
To disqualify several at once, tick them on the **Leads** list and choose **Disqualify** in the bulk bar.
Leads have no archive action; disqualifying is how you close one.
Also called: reject a lead, close a lead, archive a lead.

Disqualifying archives the lead, records a reason from an administrator-managed
list, and an optional note. The record stays fetchable. It becomes read-only:
"This lead is closed and cannot be changed."

### How do I reopen a disqualified lead?
To reopen a disqualified lead in Margince, open the lead and choose **Reopen**, then **Reopen lead** in the **Reopen this lead?** dialog.
The lead returns to the status it held before it was disqualified, and the reason is cleared. History and score are kept.
Leads disqualified in bulk have no one-step undo: reopen them one at a time.
Also called: restore a lead, undo a disqualification.

### How do I edit a lead?
To edit a lead in Margince, open it from **Leads** and click the value to change in its **Details** panel; each field changes in place, with no Edit button.
1. Click a value, change it, and press Enter or pick from the list. Esc cancels. **Email** opens a small form with **Save**.
2. Fields: **Full name**, **Email**, **Title**, **Company**, **Owner**, **Company routing key**, **Project**, **Source**, custom fields.
**LinkedIn URL** cannot be changed on a lead. Status moves on the **Lead status** ladder; score with **Override score** and a reason. A closed lead "cannot be changed".
Also called: update a lead, rename a lead, correct a prospect.

### How do I change a lead's owner?
To change or set the owner of a lead in Margince, open the lead and choose **Assign** beside **Owner** in its header, then pick a colleague under **Choose a colleague**.
1. **Assign to me** is listed first; picking it on an unassigned lead claims the lead for you.
2. Pick anyone else to hand the lead over.
The **Owner** row in **Details** does the same. For many leads, tick them on the **Leads** table and use **Choose owner** → **Assign**. The new owner must be "an active colleague you may assign work to".
Also called: assign a lead, reassign a prospect, take a lead, claim a lead.

### Scoring

Leads carry a score from 0 to 100. You can override it by hand, but a reason is
required — an override with no reason is refused, and the override stops the
score being recomputed until you clear it.

The score decays: the screen shows it as "{base}, halved every 14 days".

There is an optional first-response target, **off by default**, set between 15
minutes and 7 days. When it is on, leads show as **On time**, **Due soon** or
**Overdue**.

## Deals

A deal is created on **Deals**, on a company page, on a project page, from the
command palette, or by qualifying a lead. How a deal then moves through stages,
closes and reopens is on [The pipeline: how a deal moves](the-pipeline.md).

### How do I create a deal?
To create a deal in Margince, open **Deals** in the sidebar and choose **New deal**.
1. Enter a **Deal name** (required).
2. Pick a **Currency** (required: EUR, USD, GBP or CHF) and a **Stage** (required; only open stages are offered).
3. Optionally add a **Value**, a **Company**, a **Project** (offered once a company is chosen), partner details and **Expected close**, plus any custom fields.
4. Choose **Create**. It stays disabled until every required field is filled.
The currency is saved only when you also enter a value.
Also called: new opportunity, add a deal, open a deal.

### Where else can I start a new deal?
Besides **Deals** → **New deal**, a new deal in Margince can be started from three other places.
- The command palette (⌘K or Ctrl+K): its **New deal** action opens the form.
- A company page: **New deal** on the **Deals and projects** tab. The company is filled in; **Deal name**, **Currency** and **Stage** are required.
- A project page: **New deal** on its deals card, which files the deal under that project and its company.
On a company or project page the button is hidden when the pipeline has no open stage or the record is archived. A lead can also open a deal as it is qualified.
Also called: opportunity.

### How do I edit a deal?
To edit a deal in Margince, open it from **Deals** and click the value to change in its **Details** panel; each field changes in place, with no Edit button.
1. If the panel is hidden, choose **Show details**.
2. Click a value, change it, and press Enter or pick from the list. Esc cancels. **Value** and **Company** open a small form with **Save**.
Fields: **Deal name**, **Value**, **Currency**, **Owner**, **Company**, **Project**, **via partner**, **Forecast category**, **Expected close**, **Wait until**, **Deal brief**, **Commercial motion**, **Priority**, **Acquisition source**, custom fields. The stage moves on the **Stage** ladder.
Also called: update an opportunity, rename a deal, change deal details.

### How do I change a deal's value or currency?
To change a deal's value or currency in Margince, open the deal and click **Value** in its **Details** panel; change **Value** or **Currency** and choose **Save**.
Value and currency go together: a value with no currency is refused, and so is a currency with no value. Changing the currency keeps the figures as typed, now in the new currency; nothing is converted.
When the recurring value came from an accepted offer, the row reads **Expected ARR**, "comes from the accepted offer and is not edited here"; accepting another offer replaces it.
Also called: change the amount, update deal size, switch to dollars.

### How do I change a deal's expected close date?
To change when a deal is expected to close in Margince, open the deal and click **Expected close** in its **Details** panel, then pick the date.
An open deal cannot be given a date in the past: "an open deal cannot claim a close date in the past; pick today or later". A closed deal keeps its dates editable, past ones included.
To park a deal until a date without moving its close date, set **Wait until** instead.
Also called: push the close date, change closing date, slip a deal.

### How do I change a deal's company or project?
To change the company or project of a deal in Margince, open the deal and click **Company** in its **Details** panel; pick the **Company**, then a **Project** of that company, and choose **Save**.
A deal and its project must belong to the same company, so **Project** offers only that company's projects and empties when you change the company. **New project…** creates one on the chosen company; without a company it says "Select the deal’s company first. A project belongs to a company."
Also called: move a deal to another account, attach a deal to a project.

### How do I change the owner of a deal?
To reassign a deal to a colleague in Margince, open the deal and pick the colleague in the **Owner** row of its **Details** panel. It saves as soon as you pick.
To reassign many, switch **Deals** to **Table**, tick them, and use **Pick an owner** → **Assign** in the bulk bar.
The new owner must be "an active colleague you may assign work to": a user on a full seat, not an agent, within your own reach.
Also called: hand over a deal, change deal owner, transfer an opportunity.

### Can I edit a closed or archived deal?
A won or lost deal in Margince can still be edited: only its **Stage** ladder is locked until you reopen it. Changing a closed deal's value or currency fixes its exchange rate again.
An archived deal cannot be edited: "This deal is archived and takes no changes." Without edit rights you see "You cannot change this deal. Ask its owner to share it, or an administrator for edit rights."
Also called: change a won deal, fix a lost deal.

### How do I add a contact to a deal?
To add a contact to a deal in Margince as a stakeholder, open the deal and choose **Add stakeholder** in its **Buying committee** panel on the **Overview** tab.
1. Search for and pick the **Contact** (required). It must already exist.
2. Optionally type a **Role**, such as economic buyer, and a **Started** date.
3. Choose **Create**.
To change the role, choose **Edit** on the row; to take someone off, **Remove** ("Remove this relationship? There is no undo."). From a contact's **Deals** tab, **Add to a deal** does the same from the other end.
Also called: add a stakeholder, add a decision maker, link a contact to an opportunity.

### Can I move a deal to another pipeline?
No. A deal in Margince stays in the pipeline it was created in, and its stage can only move within that pipeline. The **Pipeline** selector on **Deals** only chooses which pipeline you are looking at. To move the business to another pipeline, create a new deal there and archive the old one.
Also called: change a deal's pipeline, switch pipeline.

### Can I set the win probability of one deal?
No. Win probability in Margince belongs to the stage (**Settings → Pipelines**, **Win probability**), not to a single deal. To say how likely one deal is, set its **Forecast category** in **Details**: Commit, Best case, Pipeline or Omitted.
Also called: deal probability, likelihood, confidence.

## Projects

> "A project is the body of work a deal is about. It starts during the deal, in
> the initiative phase, and outlives close-won: once the deal is won, delivery is
> tracked here."

A project is **not** a folder you create when you win. It is born while you are
still selling.

### How do I create a project?
To create a project in Margince, open **Projects** in the sidebar and choose **New project**.
1. Enter the **Project name** (required).
2. Pick the **Company** (required) — the client the work is for.
3. Optionally add a **Description** and a **Target end date**.
4. Fill any custom fields, then choose **Create**.
You become the project's owner; reassign it later with **More actions** → **Assign to a colleague**. The server mints the project key from the name.
Read-only seats cannot create projects.
Also called: start a new project for a client, new engagement.

### Can I create a project from a company page or a deal?
To create a project from a company page in Margince, open its **Deals and projects** tab, choose **New project** in the **Projects** panel, enter a **Project name** and choose **Create**. The project is created on that company. An archived company has no **New project**.
You can also start one while creating a deal: on **Deals** → **New deal**, choose the **Company**, then under **Project** pick **New project…** and enter a **Project name**. The project is created on the deal's company.
Also called: add a project to a client.

### What is on a project page?
A project page in Margince gathers everything filed under the work. Its phase and key sit beside the name, the **Phase** bar runs under the header, and **More actions** holds **Edit project**, **Share**, **Assign to a colleague** and **Archive project**. The main column opens with the figures **Open deals**, **Won deals**, **Open commitments** and **Activity**, then **Delivery health**, the **Deals** card with **New deal**, and **Open commitments**: open tasks filed under the project, soonest due first, late ones marked **Overdue**. The timeline follows.

The side column of a project page in Margince holds **Companies**, **Responsible**, **Stakeholders**, **Contracts**, **Documents** and **Phase history**, which records every phase move with who made it and the reason. Files attached to the project's deals stay on the deals.

### How do I edit or rename a project?
To edit or rename a project in Margince, open it from **Projects**, choose **More actions** → **Edit project**, change the fields and choose **Save**.
You can change **Project name** (required), **Owner**, **Description**, **Target end date** and custom fields. It confirms "“{name}” saved". Emptying **Description** or **Target end date** clears it.
The company is not in this form: add or take off companies in the project's **Companies** panel. The phase moves on the **Phase** bar. An archived project takes no changes.
Also called: update a project, change the project's end date.

### How do I change a project's owner?
To reassign a project in Margince, open it and choose **More actions** → **Assign to a colleague**, search under **Search colleagues**, pick one and choose **Assign**. It confirms "Assigned to {name}".
**Edit project** also has **Owner**, with **Keep current owner**, **Me** and **Unassign**. A project cannot be claimed, so an unowned one can only be changed by a role that edits every project.
Also called: hand over a project, change project lead, transfer a project.

### How do I add or remove a company on a project?
To add a company to a project in Margince, open the project and choose **Attach company** in its **Companies** panel, search under **Search companies by name**, pick a role under **As** (**Customer**, **Partner** or **Subcontractor**) and confirm with **Attach company**.
To take one off, choose **Detach** on its row, then **Remove company** in "Remove company from project?". The last company cannot be removed, nor a company that still has deals on the project.
Also called: add a partner to a project, link a client to a project.

### How do I move a project to another phase or close it?
To move a project in Margince, open it and click the phase you want on its **Phase** bar: **Initiative**, **Pursuing**, **Delivering** or **Closed**.
In **Move to {phase}**, add a **Reason** and choose **Move**. For **Closed** the button is **Close project**, and the reason is required: "A closed project needs a reason."
Click another phase on a closed project to reopen it.
Also called: close a project, finish delivery, reopen a project.

### How do I start delivery when a deal is won?
To start delivery on a won deal in Margince, open the deal and press **Start delivery** in the callout on its page.
The callout appears when the deal is won, names no project, and its company has exactly one live project: "This deal is won and names no project. Attach it to {project} and move the project into delivery?" Pressing it attaches the deal and moves the project to **Delivering**, then opens the project. A deal already on a project needs nothing: winning it moves an Initiative or Pursuing project to Delivering by itself. Also called: hand over to delivery, kick off the project.

### Phases

A project has four fixed phases: **Initiative → Pursuing → Delivering →
Closed.**

Movement is allowed in either direction — a closed project can be reopened — and
only closing needs a reason. You cannot set the phase by editing the record; you
move it.

**A won deal moves its project to Delivering by itself** — but only from
Initiative or Pursuing. A project already delivering stays put, and a closed
project is never silently reopened by a win. Reopening is a human move with a
reason.

Every phase move is recorded, including the automatic one, which is attributed to
the colleague whose action caused it.

### The key

Every project gets a short key, minted by the server. You cannot choose it.

The key comes from the project's name — initials for a multi-word name, the
first eight letters for a single-word one — plus the lowest free number. So
"Nordwind ERP rollout" becomes `NER-1`, and "ERP rollout Acme" becomes `ERA-1` —
the number counts within its own stem, not across all projects.

The key is what files email automatically. Any subject carrying it **in square
brackets** goes under that project. See [Capture](capture.md).

Keys are unique among live projects only. Archiving a project frees its key.

### How do I choose a good project key?
To get a readable project key in Margince, choose the project's name well: the key is minted from the name once, when the project is created, and never changes.
Lead with the customer, then the work: "Nordwind ERP rollout" gives `NER-1`, while "ERP rollout" gives `ER-1`, which names nobody. Three or four words beat one: "Datenmigration" gives `DATENMIG-1`. A name opening with digits drops them, and a name with too few letters falls back to `PRJ`. Renaming a project keeps its key; to get a different one, archive the project and create it again. Also called: project code, project number, ticket prefix.

### How do I get a customer's emails filed under a project?
To have a customer's emails filed under a project in Margince, get the project's key in square brackets into the subject, such as `[NER-1]`.
Send from the deal or project, or pick the project under **Project** in the composer, and Margince puts the tag at the front of your subject; a reply that keeps the subject brings the answer in. Ask the customer in your kickoff mail to keep `[NER-1]` in the subject. See [Capture](capture.md) for every filing rule. Also called: email not on the project, project reference in email.

### Companies and stakeholders

A project always has at least one company. Trying to remove the last one is
refused: "A project keeps at least one company; add another before taking this
one off." A company with deals still on the project cannot be removed either.

Companies on a project are offered three roles — **Customer, Partner,
Subcontractor** — but these are descriptive labels, not an enforced list. Two
companies can both be Customer, or none can be.

Stakeholders on a project use nine roles: **Champion, Economic buyer, Blocker,
Influencer, User, Sponsor, Project lead, Delivery lead, Subject-matter
expert.**

You can seat someone from either end — the project's own Stakeholders card adds,
removes and re-roles, and so does the contact's page. The contact page offers
the five delivery-flavoured roles; the project card offers all nine. The card
goes read-only when the project is archived, or where your grant withholds
seating.

### How do I add a stakeholder to a project?
To add a contact to a project in Margince, open the project and choose **Add stakeholder** in its **Stakeholders** card.
1. Search under **Search contacts by name** and pick the contact.
2. Pick the **Role**: one of the nine listed above.
3. Choose **Add**. A contact already on the project takes the new role instead: one role per contact.
To take someone off, choose **Remove** on the row and confirm **Remove stakeholder?**; their activity stays where it is.
Also called: add a sponsor, add a project lead, add someone to a project team.

### Who may do what
Project permissions by role:

| Role | Create | Read | Edit and move phase | Archive |
|---|---|---|---|---|
| User | yes | yes | yes | **no** |
| Team Lead | yes | yes | yes | yes |
| Management | yes | yes | yes | yes |
| Admin / Ops | yes | yes | yes | yes |
| Read-only | no | yes | no | no |

Archiving a project is final from the app: "Archiving removes this project from
the active list and frees its key. This cannot be undone here." It does **not**
archive the deals or activities underneath — the grouping dies, the history does
not.
