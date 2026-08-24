# Import a spreadsheet of companies

Bring a CSV of companies into the CRM — through an assistant over MCP, or over
REST. Two things happen here: **adding** companies you do not have, and
**correcting** ones you do.

Every import previews before it writes. The preview counts what the commit will
do, row by row, and nothing lands until you approve the run.

> **Undo covers what a run CREATED, and only that.** A signed-in human can
> reverse a completed CSV run: it archives the rows that run created and that
> nobody has touched since, leaving edited ones alone and naming them. What it
> cannot do is restore a company that existed *before* the run — a correction
> overwrites the old values and no call puts them back.

## The shape of a run

Three steps, whichever door you come through:

1. **Preview** — hand over the file and a column mapping. Writes nothing, and
   answers a `run_id` plus a report.
2. **Read the report** — the counts, and the issues naming any row that will not
   land.
3. **Commit** — approve the run by its id. *This* is the call that writes.

Over MCP that is three tool calls: `preview_import` (the CSV goes inline, as
text), `read_import_report` and `commit_import`.

Over REST the file is uploaded first, so it is four: `POST /v1/imports/sources`
with the file as multipart, then `POST /v1/imports` naming the `source_ref` it
answers, `GET /v1/imports/{id}/report`, and `POST /v1/imports/{id}/approve`.

**The upload and the undo are human-only.** Both take a signed-in session and
refuse an agent principal, so an assistant previews and commits but cannot upload
a file on your behalf or reverse a run afterwards.

## Adding companies

Name each source column and the field it feeds. The importer does not guess:

```json
{
  "object": "organization",
  "csv": "name,city,country,size\nHelios Logistik GmbH,Hamburg,DE,51-200\n",
  "mapping": {
    "name": "display_name",
    "city": "address.city",
    "country": "address.country",
    "size": "size_band"
  }
}
```

A company can receive `display_name`, `legal_name`, `industry`, `size_band`,
`description`, and the six address fields: `address.line1`, `address.line2`,
`address.city`, `address.region`, `address.postal_code`, `address.country`.
Map a name it does not take and the run is refused with the list of what it
does — before anything is written.

**A file of people becomes leads, not contacts.** Set `object` to `lead` for
those. That is a deliberate rule, not an omission: an unqualified list must not
land in the clean core.

### What happens to a company you already have

`on_duplicate` decides. It takes `create` (the default — lands a second record
and files the pair for review) or `skip` (leaves the existing one alone).

For a spreadsheet, `create` is usually the wrong choice: a hundred rows of
companies you already have becomes a hundred twins, each needing a merge. The
preview tells you how many before you commit — see `duplicates` below.

**Neither of these corrects anything.** For that, the file has to say *which*
company each row is.

## Correcting companies

Give each row the id of the company it is, and map that column to **`id`**:

```
id,display_name,city
01a02ed1-0866-7567-b567-2abcf76e5c1e,Kestrel Data,Bremen
```

A row carrying an `id` **updates that company**. No matching, no guessing — it
names one record. Read the companies out first to get their ids, edit the file,
import it back.

Three rules worth knowing:

- **An empty `id` is an ordinary create.** One file can carry corrections and new
  companies together — as long as some other column identifies every row. A file
  whose *only* identifying column is `id` needs one on every row, since a row
  with no identity cannot be re-imported or undone; see `source_key` below.
- **An id nothing answers to is refused**, and the row is reported as a skip
  saying so. It is never created quietly under a new id — a stale export or a
  typo should send you back to the file, not leave a surprise record behind.
- **Nothing is written to `id` itself.** It names the record; it is not a value
  the record holds.

### Which column identifies a row

Every row needs one column that identifies it *within your file*. It is what makes
a re-import update rather than duplicate, and what lets an undo find what a run
created. The importer uses the company name by default, and `source_key` names a
different column when your file has a better one.

Two shapes work:

- **A corrections-only file.** `id,city` is complete: the id identifies the row
  and names the record, and every row must carry one.
- **A mixed file.** Map a column every row carries — the company name will do —
  and let the `id` column be empty on the rows that are new.

A row with no identifying value at all is reported as an unusable line rather than
imported, so nothing lands that could not later be found again.

### Why not just match on the name?

Because a name does not identify a company, and the CRM is explicit about it. The
matcher that finds *likely* duplicates is built to answer "should a human look at
these two?", and it blurs on purpose to do that:

- It strips the legal form, so `Acme Inc` and `Acme GmbH` are the same string —
  and the CRM routes those to a person precisely because they are different
  companies.
- It scores a trading name against a registered one, so your row's `Kestrel Data`
  matches a company registered under that name but trading as something else.
- Two companies may legitimately share a name. Nothing stops it, and where
  several match the matcher picks one arbitrarily.

Every one of those is free when the answer is "show a human two records". Each
one is a way to overwrite the wrong company when the answer decides a write — and
that overwrite is not reversible. An id has none of them.

## Reading the report before you commit

```json
{
  "rows_read": 100,
  "disposition": {
    "created": 6,
    "updated": 88,
    "unchanged": 6,
    "skipped": 0,
    "duplicates": 94
  },
  "issues": []
}
```

- **`created`, `updated`, `unchanged` and `skipped` sum to `rows_read`.** Every
  row has exactly one outcome. If they do not add up, the report is hiding
  something.
- **`duplicates` sits outside that sum.** It is not a fifth outcome — it counts
  rows already counted elsewhere. It is the number you actually weigh: *"100
  companies, 94 of them already here"* is a different decision from *"100 new
  companies"*, and the two are indistinguishable from `created` alone.
- **`unchanged` means matched and nothing differs.** Separate from `updated` so
  that re-running a file you already applied reports no work rather than a
  hundred writes and an audit trail to match.
- **A company you are not permitted to see is not counted, not named, and does
  not change the outcome.** Your row is created, exactly as it would be if no such
  company existed — and a company you *can* see is still reported, even when a
  hidden one matched more closely. Skipping on the hidden one would tell you it
  exists: "your row was not created" is an answer to "is this company in your
  CRM", and that is not a fact about somebody else's private record you get to
  learn. The cost is a duplicate the review queue picks up; the cost of the
  alternative is a disclosure no merge undoes.

**`issues` names any row that will not land**, in terms of the file rather than
the database — an unusable `size_band`, an id nothing answers to. Fix those in
the source and preview again.

## Committing

Approve the run by its id. The commit is checkpointed and idempotent on the
source key, so a re-run of the same file converges rather than duplicating, and a
run interrupted midway resumes from where it stopped rather than starting over.

A correction is not recorded as something the run created, which is what keeps
`undo` honest: it archives the companies a run added, never one that was already
there and got edited.

The report keeps its shape afterwards: the same fields report what the run *did*.

## Asking an assistant to do it

Over MCP this is one instruction:

> Import this CSV as organizations. Tell me how many are new and how many are
> already here before you commit anything.

Or, for a corrections file:

> Read out our companies with their ids, then import this CSV — each row carries
> the id of the company it updates.

The assistant runs on your passport, so it can do what you could do unaided in
the app, and the import commits without a separate approval step. What still
binds it is what binds you: your seat, your grants, your row scope, and the
scopes you lent when you minted the passport. See
[connect-an-mcp-client.md](connect-an-mcp-client.md).

Two caveats. An installation can require confirmation for `commit_import`, in
which case the commit stages for a human like any confirm-first call. And the
assistant cannot upload a file for you or undo a run — both need your own
signed-in session — so over MCP the CSV goes across as text in the request.

## Related

- [import-your-linkedin-network.md](import-your-linkedin-network.md) — a
  different importer for a different file, landing relationship edges rather
  than companies.
- [connect-an-mcp-client.md](connect-an-mcp-client.md) — connecting an assistant.
- [mint-a-passport.md](mint-a-passport.md) — issuing the credential it uses.
