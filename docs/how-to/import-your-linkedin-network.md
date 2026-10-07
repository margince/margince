<!-- prose:plain -->
# Import your LinkedIn network

Upload your own `Connections.csv`, so Margince can answer one question about an account: **does someone
here already know someone who works there?** The steps are in the app, with the same `curl` call next to
each, for scripts and checks. To learn how it works, read
[explanation/relationship-graph.md](../explanation/relationship-graph.md) first. It covers participants, the
interaction edge, and where this second tier of evidence sits next to real interaction history.

> **Your network, not the company's.** The owner of every imported row is the **signed-in caller**,
> never a field in the file. No one can upload the network of another user for them, because
> *"Lars knows them"* has to mean Lars.

## What this is not

- **Not a LinkedIn integration.** There is no LinkedIn app to register, no OAuth app to approve, and no API
  access to get. This is your own data export, read by the importer.
- **Not scraping.** Nothing here calls LinkedIn at all. You download the file, and you upload the file.
- **Not a contact import.** The rows never become contacts. See
  [What the imported rows are](#what-the-imported-rows-are).

**The onboarding LinkedIn card only saves your profile.** The connect step asks for your profile URL, and
stores it with `PUT /me/linkedin-account` (`connected` stays false). So the network you import here is
yours: *"Anna knows them"*, never *"the company knows them"*. The card gives no access and gets nothing;
its form says so and points here. The Member Data Portability API is planned as a second writer to the same rows,
once an app is approved. The CSV path is the one that works now.

## Step 1: Get the file from LinkedIn

LinkedIn gives every member their own export, with no approval needed:

1. On LinkedIn, go to **Settings** → **Data privacy** → **Get a copy of your data**.
2. Ask for the archive, and wait for the download link.
3. Open the archive, and take `Connections.csv`. The archive holds about 12 other files, and the wrong one fails
   with a parse error that explains nothing.

Do not edit the file. The importer finds the header by its **content**, and not by its place in the file.
The export format changes, and is not the same in each language (the importer knows English and German
headers). It allows the `Notes:` text that LinkedIn puts above the header. The importer refuses a file with
no header row it knows as `422 unreadable_export`, and does not import part of it.

## Step 2: Import it

**Settings → Integrations → LinkedIn connections.**

1. If you want, paste **your LinkedIn profile URL** and click **Save profile**.

   This puts your name on the network, so the CRM says "Anna knows them", and not "the company knows them".
   An empty value *clears* the stored URL.
2. Click **Choose Connections.csv** and choose the file. The upload starts as soon as you choose it.
3. Read the counts.

   They are **Connections imported**, **Matched to a contact**, **Awaiting your confirmation**, and (only
   when it is not zero) **Rows skipped (no usable name)**.

The LinkedIn card in the onboarding connect step only records your profile and consent. The import happens
in Settings.

<details><summary>Same thing via <code>curl</code></summary>

The endpoint takes `multipart/form-data`, with a part named `file`. It is `x-agent-access: human-only`: it
takes a session cookie, and refuses an agent passport. The upload can be **8 MB at most by default**. That is
enough for many rows of short text. It still refuses a wrong file, such as a video, before it
reaches the CSV reader. The operator of the installation sets the real number
(`uploads.linkedin_import_mb` in `margince.yaml`), and the error names the limit that applies.

```sh
curl -X POST http://localhost:8080/v1/me/linkedin-connections \
  --cookie 'crm_session=<session>' \
  -F 'file=@Connections.csv;type=text/csv' \
  | jq '{rows, imported, skipped, confirmed, suggested}'

# your own profile row, and the connection count it yielded
curl --cookie 'crm_session=<session>' http://localhost:8080/v1/me/linkedin-account \
  | jq '{connected, connected_at, profile_url, connections}'
```

`confirmed` and `suggested` in the answer are your **totals**, and not the change from this upload. The
matcher looks only at rows that no one has decided on. So when you import the same export again, it reports
zero *new* matches, and the "Matched to a contact" card still shows the total.
</details>

## What the imported rows are

They are **ghosts**: data for the graph, and nothing else. An export is a list of third parties who never
agreed to be in any CRM. To turn them into contacts would be a consent problem and a problem with the data,
at once. The migration that made the table states this as the rule that keeps the feature safe.

An imported row:

| | |
|---|---|
| **Does not show in** | search, lists, the contacts screens, and the record tools of the assistant |
| **Takes no writes** | no timeline, no activities, no fields: nothing can write to a ghost |
| **Cannot be reached by outreach** | no email, no sequence, and no send path finds one |
| **Is owned by** | the signed-in caller, always: `POST /me/linkedin-connections`, never `/users/{id}/…` |
| **Exists to answer** | one question: does someone here already know someone at this company |

When you **confirm** a match, the profile URL of *the connection itself* goes on that contact, as a
`linkedin` handle. That is the only thing a ghost gives to a real record. Its name, employer, title and
connection date stay where they are.

Your own URL is never used, because it would put the wrong address on
every contact you confirm. A contact that already has a `linkedin` handle keeps it. Someone stated that
handle, and a match is no reason to change it. The answer tells you which case happened.

## What matching decides

Matching runs at once after the upload, so the answer can say what your import matched. It follows the rule
that the rest of the contacts module follows: **only an email address is an exact key.**

| Evidence | Outcome | Why |
|---|---|---|
| **Exact email** matches the address of a contact | **Confirmed on its own** | An address is identity here, as it is on the capture path. To ask a human to confirm it again would add a step that the system already handles in every other place. |
| **Exact name + matched employer**, no other candidate | **Confirmed on its own** | The names are the same, the employer agrees, and no one else here has that name. To ask about these makes users click through the queue without reading, and then the open cases become a risk. |
| **Folded name + matched employer** ("André" or "Andre") | **Suggested**: goes to the approval inbox | Only a human can decide whether two spellings are one contact. |
| **Two matching names**: two contacts with the same name at the same employer | **Nothing** | To choose one would be a guess recorded as a confirmation. |
| Name only, no employer match | **Nothing** for the contact | A name alone is not enough to match. |

The matcher also applies these rules:

- The employment must be **live today**: `archived_at IS NULL` and `(ended_at IS NULL OR ended_at > today)`.
  An end date after today is still employment: a contact who leaves next month is still at work today.
- It will not suggest a contact that another of your ghosts is already confirmed against. One contact cannot
  be two different LinkedIn connections of the same colleague, and to offer both asks for a wrong
  click.

**Nothing here ever creates a contact.** A ghost that matches nothing stays a ghost. All it gives is the
count for the account, which needs no identity at all.

### Where the suggestions go

A match that a human has to decide is an **approval**. Suggested matches go into the normal approval inbox
as `linkedin_match` proposals, one for each match, and not one for each import. Each decision stands alone,
and one proposal for the whole batch would make you take 30 links to get the three you wanted. The
proposal carries the spelling of the connection from the export (name and employer), and the contact it is
matched against. That is what you decide the guess on.

**Any user with access can decide one**; it is not kept for you. The subject of the proposal is a contact
already on file. The inbox only shows a proposal to a user who can already see that contact.

To decide one takes the normal two things: the right to write the contact, and the right to see it. A colleague
learns only that a contact already in the CRM is in the network of someone. The network card of that contact
already shows that. The user who decides it does not change where the link goes. It goes on the network of the export, never on the
network of the user who decides.

A refusal **lasts**. The approval row stays, and the matcher skips a ghost that already has a decided
proposal. So when you refuse "André is Andre" one time, no one asks you again, even after you import a new
export.

## Why nothing matched yet

Zero matches on a new workspace is normal. You upload your export at onboarding. The contacts and
accounts it *could* match come in over the next hours, as mail capture runs.

Two ways close that gap, and you do not need to upload again:

- **The event path** (`cg:linkedin-match`). `contact.created`, `contact.updated` and the company events
  already reach the outbox, because the write shape puts them there. A record a user types in, capture, a
  site read, a merge and an import all start a new match. No part of it needs to know the matcher exists.
  Company events count most. Most ghosts with no match wait on an *employer*, not a name, so a new account
  can match a whole batch at once.
- **The `linkedin_rematch` sweep**, which runs for each workspace **every hour**. It runs each hour, and not
  each day, because it covers the first day of a workspace. An export uploaded at onboarding waits on a
  capture backfill that ends in minutes. A rep who imported their network in the morning should see it on an
  account the same day.

Both passes look only at ghosts with **no match**, so they never look again at a confirmation or a refusal.
A workspace with nothing new costs one query. Both run with **your own** rights, and not the rights of
the system. The system can see all data. With its rights, a CSV of one row would turn into a way to learn
secrets. Upload a guessed address, wait, read the match status, and learn whether a contact you cannot see
exists.

## What you get: where your network reaches

**Settings → Integrations → Where your network reaches** (`GET /me/linkedin-reach`) is the answer for each
account that the import is for. For each account it reports:

| Column | Meaning |
|---|---|
| **Account** | the company, with a link to its company page |
| **You know** | how many of *your* connections work there |
| **Already contacts** | how many of these are confirmed matches, in the form `{on file} of {total}` |

The gap between the two columns is the finding: **connections you know at this account with no confirmed
contact match**. A suggested connection, or one with no match, may still be a contact on file; only a
confirmed match counts. Rows are ranked by connection count, then name, then id, so two reads of the same network
return the same order. A note under the table states what the view cannot show. It says
how many accounts are past the page limit, and how many connections matched no account at all.

<details><summary>Same thing via <code>curl</code></summary>

```sh
curl --cookie 'crm_session=<session>' 'http://localhost:8080/v1/me/linkedin-reach?limit=25' \
  | jq '{accounts_total, unresolved_connections,
         accounts: [.accounts[] | {display_name, connections, contacts_on_file}]}'
```
</details>

### Why the reach total and the import total are not the same

They count different things:

- The **import summary** counts rows in your file: `rows`, `imported`, `skipped`.
- The **reach view** counts only connections **placed at a company you can read**. All other connections go
  to `unresolved_connections`.

A connection is "unresolved" when its employer matched no account on file, or the employer text was of no
use. It is also "unresolved" when the employer matched an account **outside your row scope**. The view
cannot tell these cases from each other, so it does not show accounts outside your row scope.

Say it counted that
last case on its own. Then you could take one number from the other, to show that an employer matched
something you may not see. If you upload one row for each guessed company name, you could list these
accounts. As "unresolved", it looks like a company that no one here has on file. That is the answer every
list in the product with a row scope gives.

`accounts_total` counts every account reached, not only the page returned, so a list that ends at the page limit does not
show a smaller reach.

## Import a new export again

To import again **updates** rows, and does not make copies, so the reach counts keep their meaning when users
export again. The upsert:

- Keys on `(owner, normalized name, normalized company, connected-on date)` for CSV rows. This is a dedupe
  key, not an identity claim, and it can be wrong. The connection date is part of it. Two
  connections with the same name at one company most likely do not connect on the same day.
- Fixes stale keys **before** the upsert. The system works out `normalized_company`, a part of that key.
  So rows that an older version wrote would no longer match what the current import works out.
- Lets a **later export win on the profile URL**. Someone who changed their custom address can be reached at
  the new one. It keeps the email it has when the new row has no email.
- **Makes live again** a connection that an earlier export dropped, and clears its tombstone.

Rows the importer cannot use are **counted, never dropped without a count**:

| What you see | What happened |
|---|---|
| `skipped > 0` | Rows with no usable name. They name no one, so they are counted and skipped. |
| `imported < rows - skipped` | A row matched the **erasure block list** by address and was refused. So an erased subject cannot come back through the next export of a colleague, and so work against an Art. 17 request. It is not reported as imported, because the erasure made the system delete that data. |
| `422 unreadable_export` | No LinkedIn header row the importer knows. Export the file from LinkedIn, and do not edit it. |
| `422 invalid_multipart` / `422 required` | Not sent as `multipart/form-data`, or the `file` part is missing. |
| `413 body_too_large` | Over the upload limit of this installation. The message names the limit that applies; the default is 8 MB. |

## When rows are deleted, and privacy

- **Turning off your account deletes your connections.** The transaction that turns off
  the account deletes every `linkedin_connection` you own, and also revokes your session and passport. The
  rows are *deleted*, and not marked with a tombstone, because a tombstone still holds the names.
- **Erasing a subject deletes their imported rows.** An Art. 17 erasure deletes the ghosts of
  the subject, in the same transaction as the rest of the erase. It matches on evidence at the level of a
  suggestion. That is a row matched to them, or with their address, or with their LinkedIn URL. It is also a row with their name at an
  employer they work for. See [explanation/privacy-and-consent.md](../explanation/privacy-and-consent.md).
- **Your network is yours.** Every action in this guide is `/me/…` and `x-agent-access: human-only`. No API
  path leads to the LinkedIn account or connections of another member, for any seat, **admin too**. No
  agent passport can drive any of it.
- A **suggested match** becomes an approval, and the normal rule of the inbox decides who may decide it.
  That is the grants the change needs, and the right to see the **contact** the proposal is about. A
  colleague who can already read that contact can see that one proposal, with the spelling of the
  name and employer of the connection.
- The **audit row and the outbox event** for an import name **no connection at all**, only `rows`,
  `imported` and `skipped`. When you save your profile, the URL goes in *your own* audit row, but stays out of
  the event that goes out to others.

## Check it from end to end

1. **Check that the file was read.** The result card shows `Connections imported` > 0.

   `Rows skipped` matches what you expect from the state of the export.
2. **Check that nothing turned into a contact.** Search for the name of an imported connection.

   Search in the contacts screen and in the search for the whole app: no result. `GET /contacts` does not list them.
3. **Check that matching followed the rule.** A connection whose address is already on a contact shows as confirmed.

   A connection with the same name but another spelling waits in the **approval inbox** as a
   `linkedin_match` proposal, and not in a settings queue.
4. **Check that a refusal lasts.** Refuse one proposal, upload the same file again, and confirm that no one
   asks you again.
5. **Check that a new import makes no copies.** Upload the same file twice.

   `Connections imported` stays the same in both runs, and is not two times the first count. **Where your network
   reaches** shows the same counts.
6. **Check the unresolved total.** Use a new workspace with no accounts.

   The reach card says that all *N* of your connections work at a place that is not an account on file
   yet. It does not say `none yet`.
7. **Check that matches come as the CRM fills.** Let mail capture run, or create a contact by hand at
   a matching employer.

   Confirm that the count changes without a new upload, within the hour at most.

## Where to go next

- The model this page adds data to (participants, the interaction edge, warmth, and deal coverage):
  [explanation/relationship-graph.md](../explanation/relationship-graph.md).
- Where the evidence that counts most comes from, when you connect a mailbox so real interaction history exists:
  [connect-a-mailbox.md](connect-a-mailbox.md) and
  [explanation/capture-connectors.md](../explanation/capture-connectors.md).
- What happens to all of this under an Art. 17 request:
  [explanation/privacy-and-consent.md](../explanation/privacy-and-consent.md).
