# Import your LinkedIn network

Upload your own `Connections.csv` so Margince can answer one question about an account: **does anybody
here already know somebody who works there?** The steps are in the UI, with the equivalent `curl`
shown alongside for scripting and verification. For the mental model (participants, the interaction
edge, and where this weaker evidence tier sits beside real interaction history), read
[explanation/relationship-graph.md](../explanation/relationship-graph.md) first.

> **Your network, not the company's.** The owner of every imported row is the **authenticated caller**,
> never a field in the file. There is no way to upload somebody else's network on their behalf, because
> *"Lars knows them"* has to mean Lars.

## What this is not

- **Not a LinkedIn integration.** There is no LinkedIn app to register, no OAuth app to approve, no API
  to be granted. This is your own data export, read by the importer.
- **Not scraping.** Nothing here talks to LinkedIn at all. You download the file; you upload the file.
- **Not a contact import.** The rows never become contacts. See
  [What the imported rows are](#what-the-imported-rows-are).

**The onboarding LinkedIn card only saves your profile**. The connect scene asks for your profile URL
and stores it with `PUT /me/linkedin-account` (`connected` stays false), so the network you import
here is attributed to you: *"Anna knows them"*, never *"the company knows them"*. It authorizes nothing
and fetches nothing; the dialog says so and points here. The Member Data Portability API is designed as
a second writer onto the same rows once an app is approved; the CSV path is the one that works now.

## Step 1: Get the file from LinkedIn

LinkedIn hands every member their own export, no approval involved:

1. On LinkedIn, go to **Settings** → **Data privacy** → **Get a copy of your data**.
2. Request the archive and wait for the download link.
3. Unzip it. **The file you want is `Connections.csv`.** The archive holds a dozen others, and picking
   the wrong one fails with a parse error that explains nothing.

Don't edit the file. The importer recognizes the header by its **content** rather than by position,
because the export format changes and differs by locale (English and German headers are both
recognized). It tolerates the `Notes:` preamble LinkedIn puts above the header. A file with no
recognizable header row is refused as `422 unreadable_export` rather than half-imported.

## Step 2: Import it

**Settings → Integrations → LinkedIn connections.**

1. Optionally paste **your LinkedIn profile URL** and click **Save profile**. This attributes the
   network to you by name, so the CRM says "Anna knows them" rather than "the company knows them".
   An empty value *clears* the stored URL.
2. Click **Choose Connections.csv** and pick the file. The upload starts on selection.
3. The result appears as counts: **Connections imported**, **Matched to a contact**, **Awaiting
   your confirmation**, and (only when non-zero) **Rows skipped (no usable name)**.

The onboarding connect scene's LinkedIn card only records your profile and consent. Settings is where
the import happens.

<details><summary>Same thing via <code>curl</code></summary>

The endpoint is `multipart/form-data` with a part named `file`, and it is
`x-agent-access: human-only`: it takes a session cookie, and an agent Passport is refused. The upload
is bounded at **8 MB by default**, enough for a few thousand rows of short text while still refusing
a mis-picked video before it reaches the CSV reader. Whoever operates the installation sets the real
number (`uploads.linkedin_import_mb` in `margince.yaml`), and the refusal names the one in force.

```sh
curl -X POST http://localhost:8080/v1/me/linkedin-connections \
  --cookie 'crm_session=<session>' \
  -F 'file=@Connections.csv;type=text/csv' \
  | jq '{rows, imported, skipped, confirmed, suggested}'

# your own profile row, and the connection count it yielded
curl --cookie 'crm_session=<session>' http://localhost:8080/v1/me/linkedin-account \
  | jq '{connected, connected_at, profile_url, connections}'
```

`confirmed` and `suggested` in the response are your **totals**, not this pass's delta. The matcher
only considers rows nobody has decided on, so re-importing the same export reports zero *new* matches,
and the "Matched to a contact" card still shows the total.
</details>

## What the imported rows are

They are **ghosts**: graph substrate, and nothing else. An export is a list of third parties who never
agreed to be in anyone's CRM, so turning them into contacts would be a consent problem and a
data-quality problem at once. The migration that created the table states this as the feature's
safety property.

An imported row:

| | |
|---|---|
| **Is invisible to** | search, lists, the contacts screens, and the assistant's record tools |
| **Cannot be written to** | no timeline, no activities, no fields: nothing can write to a ghost |
| **Cannot be reached by outreach** | no email, no sequence, no send path resolves one |
| **Belongs to** | the authenticated caller, always: `POST /me/linkedin-connections`, never `/users/{id}/…` |
| **Exists to answer** | one question: does anyone here already know someone at this company |

When you **confirm** a match, the *connection's own* profile URL is written to that contact as a
`linkedin` handle. That is the only thing a ghost contributes to a real record; its name, employer,
position and connection date stay where they are. Your own URL is never used, since it would put the
wrong address on every contact you confirm. A contact that already carries a `linkedin` handle keeps
it, because that handle is somebody's statement and a match is no grounds to replace it. The response
tells you which happened.

## What matching decides

Matching runs immediately after the upload, so the response can say what your import achieved. It
follows the rule the rest of the contacts module obeys: **only an email address is an exact contact
key.**

| Evidence | Outcome | Why |
|---|---|---|
| **Exact email** matches a contact's address | **Confirmed automatically** | An address is identity here, as it is on the capture path. Asking a human to re-confirm it would add a step the system already settles everywhere else. |
| **Exact name + matched employer**, no other candidate | **Confirmed automatically** | The names are identical, the employer agrees, and nobody else here has that name. Asking about these trains users to click through the queue without reading, which makes the uncertain ones dangerous. |
| **Folded name + matched employer** ("André" vs "Andre") | **Suggested**: goes to the approval inbox | Whether two spellings are one contact needs a human's judgement. |
| **Ambiguous name**: two contacts of the same name at the same employer | **Nothing** | Picking one would be a guess recorded as a confirmation. |
| Name only, no employer match | **Nothing** on the contact side | A name alone is not enough to match. |

The matcher also applies these rules:

- The employment must be **live today**: `archived_at IS NULL` and `(ended_at IS NULL OR ended_at >
  today)`. A future end date is still employment: a contact leaving next month is at their desk today.
- It will not propose a contact that another of your ghosts is already confirmed against. One contact
  cannot be two different LinkedIn connections of the same colleague, and offering that choice invites a
  wrong click.

**Nothing here ever creates a contact.** A ghost that matches nothing stays a ghost, and its only
contribution is the account-level count, which needs no identity at all.

### Where the suggestions go

A match a human has to judge is an **approval**. Suggested matches stage into the ordinary approval
inbox as `linkedin_match` proposals, one per match rather than one per import: the decisions are
independent, and a batch proposal would force you to take thirty links to get the three you wanted.
The proposal carries the export's own spelling of the connection (name and employer) plus the contact
it is proposed against, which is what you judge the guess on.

**Anyone with access can decide one**; it is not held for you. The proposal's subject is a contact
already on file, and the inbox only shows a proposal to somebody who can already see that contact.
Deciding one takes the ordinary two things: the contact write, and being able to see the contact. A
colleague learns only that a contact already in the CRM appears in somebody's network, which that
contact's own network card already shows. Whoever decides it, the link is written against the network
the export came from, never against the decider's.

Rejection is **durable**. The approval row persists and the matcher skips a ghost that already carries a
decided proposal, so refusing "André is Andre" once means never being asked again, including after a
re-import of a refreshed export.

## Why nothing matched yet

Zero matches on a fresh workspace is expected. Your export is uploaded during onboarding; the contacts
and accounts it *could* match arrive over the following hours as mail capture runs.

Two mechanisms close that gap, and neither needs you to re-upload:

- **The event path** (`cg:linkedin-match`). `contact.created`, `contact.updated` and the company
  events already reach the outbox because the write shape puts them there. Manual entry, capture, a
  site read, a merge and an import all trigger a re-match without any of them knowing the matcher
  exists. Company events matter most: most unmatched ghosts are waiting on an *employer* rather than a
  name, so an account appearing unblocks a batch at once.
- **The `linkedin_rematch` sweep**, which fans out per workspace **every hour**. It runs hourly rather
  than daily because it covers a workspace's first day. An export uploaded during onboarding waits on a
  capture backfill that finishes in minutes, and a rep who imported their network in the morning should
  see it on an account the same day.

Both passes look only at **unmatched** ghosts, so a confirmation or a rejection is never revisited and a
caught-up workspace costs one query. Both run under **your own** authority rather than a system
principal's. A system principal is unbounded, which would turn a one-row CSV into an oracle: upload a
guessed address, wait, read the match status, and learn whether a contact you cannot see exists.

## The payoff: where your network reaches

**Settings → Integrations → Where your network reaches** (`GET /me/linkedin-reach`) is the account-level
answer the import is for. Per account it reports:

| Column | Meaning |
|---|---|
| **Account** | the company, linking to its company page |
| **You know** | how many of *your* connections work there |
| **Already contacts** | how many of those are confirmed matches, shown as `{on file} of {total}` |

The gap between the two columns is the finding: **connections you know at this account who are not in the
CRM**. Rows are ranked by connection count, then name, then id, so two reads of an unchanged network
return the same order. A footnote states what the view cannot show: how many accounts were truncated
by the page limit, and how many connections resolved to no account at all.

<details><summary>Same thing via <code>curl</code></summary>

```sh
curl --cookie 'crm_session=<session>' 'http://localhost:8080/v1/me/linkedin-reach?limit=25' \
  | jq '{accounts_total, unresolved_connections,
         accounts: [.accounts[] | {display_name, connections, contacts_on_file}]}'
```
</details>

### Why the reach total and the import total differ

They count different things:

- The **import summary** counts rows in your file: `rows`, `imported`, `skipped`.
- The **reach view** counts only connections that were **placed at a company you can read**.
  Everything else lands in `unresolved_connections`.

A connection is "unresolved" when the employer matched no account on file, when the employer string was
unusable, or when the employer resolved to an account **outside your row scope**. The view cannot tell
these apart, so it does not reveal accounts outside your row scope. If it counted that last case
separately, the two numbers could be subtracted to show that an employer resolved to something you may
not see, and uploading one row per guessed company name would enumerate accounts. Counted as
unresolved, it looks like a company nobody here has on file, which is the answer every row-scoped list
in the product gives.

`accounts_total` counts every account reached, not only the page returned, so a truncated list does not
understate reach.

## Re-importing a refreshed export

Re-importing **updates** rather than duplicates, so the reach counts stay meaningful when users
re-export. The upsert:

- Keys on `(owner, normalized name, normalized company, connected-on date)` for CSV rows. This is a
  best-effort dedupe key, not an identity claim. The connection date is in it because two same-named
  connections at one company almost certainly did not connect on the same day.
- Repairs stale keys **before** upserting. `normalized_company` is a derived part of that key, so rows
  written under an older normalizer would no longer collide with what the current import computes.
- Lets a **later export win on the profile URL** (someone who changed their vanity address is reachable
  at the new one) and keeps an existing email when the new row has none.
- **Revives** a connection an earlier export had dropped, by clearing its tombstone.

Unusable rows are **counted, never dropped without a count**:

| What you see | What happened |
|---|---|
| `skipped > 0` | Rows with no usable name. They identify nobody, so they are counted and passed over. |
| `imported < rows - skipped` | A row matched the **erasure suppression list** by address and was refused, so an erased subject cannot come back through a colleague's next export and undo an Art. 17 request. It is not reported as imported, because the system destroyed that data to honour the erasure. |
| `422 unreadable_export` | No recognizable LinkedIn header row. Export the file from LinkedIn without editing it. |
| `422 invalid_multipart` / `422 required` | Not sent as `multipart/form-data`, or missing the `file` part. |
| `413 body_too_large` | Over this installation's upload limit. The message names the limit in force; the default is 8 MB. |

## Lifecycle and privacy

- **Deactivating your account deletes your imported connections.** The deactivation transaction deletes
  every `linkedin_connection` you own, together with your session and passport revocation. Rows are
  *deleted*, not tombstoned, because a tombstone still holds the names.
- **Erasing a subject deletes their imported rows.** An Art. 17 erasure deletes the subject's ghosts in
  the same transaction as the rest of the cascade. It matches on **suggestion-grade** evidence: matched
  to them, carrying their address, carrying their LinkedIn URL, or bearing their name at an employer
  they work for. Details in [explanation/privacy-and-consent.md](../explanation/privacy-and-consent.md).
- **Your network is yours.** Every operation in this guide is `/me/…` and `x-agent-access: human-only`:
  there is no API path to another member's LinkedIn account or connections, for any seat, **admin
  included**, and no agent passport can drive any of it.
- A **suggested match** becomes an approval, and the inbox's ordinary rule decides who may decide it:
  the grants the effect needs, plus visibility of the **contact** the proposal is about. A colleague who
  can already read that contact can see that one proposal, including the connection's own spelling of
  their name and employer.
- The **audit row and the outbox event** for an import name **no connection at all**, only `rows`,
  `imported` and `skipped`. Saving your profile records the URL in *your own* audit row but keeps it out
  of the fanned-out event.

## Verify end-to-end

1. **The file was read.** The result card shows `Connections imported` > 0, and `Rows skipped` matches
   your expectation of how ragged the export was.
2. **Nothing became a contact.** Search for an imported connection's name in the contacts screen and in
   the global search: no result. `GET /contacts` does not list them.
3. **Matching obeyed the rule.** A connection whose exported address is already a contact's address
   shows as confirmed; a same-name-different-spelling one is waiting in the **approval inbox** as a
   `linkedin_match` proposal, not in a settings queue.
4. **A rejection sticks.** Reject one proposal, re-upload the same file, and confirm you are not asked
   again.
5. **Re-import does not duplicate.** Upload the same file twice; `Connections imported` stays flat
   across the two runs rather than doubling, and **Where your network reaches** shows the same counts.
6. **The unresolved total is complete.** On a fresh workspace with no accounts, the reach card says all
   *N* of your connections work somewhere that is not an account on file yet, rather than "none yet".
7. **Matches appear as the CRM fills up.** Let mail capture run (or create a contact by hand at a
   matching employer) and confirm the count moves without re-uploading, within the hour at worst.

## Where to go next

- The model this feeds (participants, the interaction edge, warmth, and deal coverage):
  [explanation/relationship-graph.md](../explanation/relationship-graph.md).
- Where the *strong* evidence tier comes from, by connecting a mailbox so real interaction history
  exists: [connect-a-mailbox.md](connect-a-mailbox.md) and
  [explanation/capture-connectors.md](../explanation/capture-connectors.md).
- What happens to all of this under an Art. 17 request:
  [explanation/privacy-and-consent.md](../explanation/privacy-and-consent.md).
