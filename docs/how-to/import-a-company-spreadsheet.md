# Import a spreadsheet of companies

Bring a CSV of companies into the CRM through an assistant over MCP, or over
REST. A run can **add** companies you do not have and **correct** ones you do.

Every import previews before it writes. The preview counts what the commit will
do, row by row, and nothing lands until you approve the run.

> **Undo covers only what a run created.** A signed-in human can
> reverse a completed CSV run: it archives the rows that run created and that
> nobody has touched since, leaving edited ones alone and naming them. It
> cannot restore a company that existed *before* the run: a correction
> overwrites the old values and no call puts them back.

## The shape of a run

The steps are the same over MCP and REST:

1. **Preview**: hand over the file and a column mapping. Writes nothing, and
   answers a `run_id` plus a report.
2. **Read the report**: the counts, and the issues naming any row that will not
   land.
3. **Commit**: approve the run by its id. *This* is the call that writes.

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
  "object": "company",
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
`description`, `domain`, `author`, and the six address fields: `address.line1`,
`address.line2`, `address.city`, `address.region`, `address.postal_code`,
`address.country`. Map a name it does not take and the run is refused, before
anything is written, with the list of what it does take.

`author` is who created the row in the system the file came from. Margince keeps
the cell as written. If the cell is the email of a Margince user, the record also
links to that user, so it shows their current name. The author is set when the
import creates the record. A later file does not change it.

**A file of contacts takes `lead` or `contact`.** Pick by where it came from. A
machine-sourced list (a scraped export, a purchased list, a badge dump) takes
`lead`: its rows land unworked, and someone promotes the ones worth keeping. A
file of contacts the business already knows (a migration off another CRM, a
customer list from a retired system) takes `contact`: those rows were qualified
elsewhere, so they land as contacts, through the same duplicate checks.

### What happens to a company you already have

`on_duplicate` decides. It takes `create` (the default: lands a second record
and files the pair for review) or `skip` (leaves the existing one alone).

For a spreadsheet, `create` is usually the wrong choice: a hundred rows of
companies you already have becomes a hundred twins, each needing a merge. The
preview tells you how many before you commit; see `duplicates` below.

**Neither of these corrects anything.** For that, the file has to say *which*
company each row is.

## Correcting companies

Give each row the id of the company it is, and map that column to **`id`**:

```
id,display_name,city
01a02ed1-0866-7567-b567-2abcf76e5c1e,Kestrel Data,Bremen
```

A row carrying an `id` **updates that company**. The id names one record, so
there is no matching or guessing. Read the companies out first to get their ids, edit the file,
import it back.

Rules for the `id` column:

- **An empty `id` is an ordinary create.** One file can carry corrections and new
  companies together, as long as some other column identifies every row. A file
  whose *only* identifying column is `id` needs one on every row, since a row
  with no identity cannot be re-imported or undone; see `source_key` below.
- **An id nothing answers to is refused**, and the row is reported as a skip
  saying so. It is never created under a new id: a stale export or a
  typo sends you back to the file and leaves no surprise record behind.
- **Nothing is written to `id` itself.** It names the record and is not a value
  the record holds.

### Which column identifies a row

Every row needs one column that identifies it *within your file*. It is what makes
a re-import update rather than duplicate, and what lets an undo find what a run
created. The importer uses the company name by default, and `source_key` names a
different column when your file has a better one.

Two shapes work:

- **A corrections-only file.** `id,city` is complete: the id identifies the row
  and names the record, and every row must carry one.
- **A mixed file.** Map a column every row carries (the company name will do)
  and let the `id` column be empty on the rows that are new.

A row with no identifying value at all is reported as an unusable line rather than
imported, so nothing lands that could not later be found again.

### Why not just match on the name?

A name does not identify a company. The matcher that finds *likely* duplicates
answers whether a human should look at a pair, and it blurs names so it can:

- It strips the legal form, so `Acme Inc` and `Acme GmbH` are the same string.
  The CRM sends those to a human to review because they can be different
  companies.
- It scores a trading name against a registered one, so your row's `Kestrel Data`
  matches a company registered under that name but trading as something else.
- Two companies may legitimately share a name. Nothing stops it, and where
  several match the matcher picks one arbitrarily.

Each of those is harmless when the result is "show a human two records". When
the result decides a write, each one can overwrite the wrong company, and that
overwrite is not reversible. An id has none of these failure modes.

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
  row has one outcome. If they do not add up, the report is missing
  something.
- **`duplicates` sits outside that sum.** It counts rows already counted under
  another outcome. It is the number to weigh: *"100 companies, 94 of them
  already here"* is a different decision from *"100 new companies"*, and
  `created` alone cannot tell them apart.
- **`unchanged` means matched and nothing differs.** Separate from `updated` so
  that re-running a file you already applied reports no work rather than a
  hundred writes and an audit trail to match.
- **A company you cannot see is not counted or named, and does not change the
  outcome.** Your row is created as it would be if no such company existed. A
  company you *can* see is still reported, even when a hidden one matched more
  closely. Skipping on the hidden one would tell you that it exists, which is a
  fact about somebody else's private record. The cost is a duplicate that the
  review queue picks up; skipping would cost a disclosure that no merge undoes.

**`issues` names any row that will not land**, in terms of the file rather than
the database: an unusable `size_band`, an id nothing answers to. Fix those in
the source and preview again.

## Committing

Approve the run by its id. The commit is checkpointed and idempotent on the
source key, so a re-run of the same file converges rather than duplicating, and a
run interrupted midway resumes from where it stopped rather than starting over.

A correction is not recorded as something the run created, so `undo` archives
only the companies a run added, never one that was already there and got
edited.

The report keeps its shape afterwards: the same fields report what the run *did*.

## Asking an assistant to do it

Over MCP this is one instruction:

> Import this CSV as companies. Tell me how many are new and how many are
> already here before you commit anything.

Or, for a corrections file:

> Read out our companies with their ids, then import this CSV. Each row carries
> the id of the company it updates.

The assistant runs on your passport, so it can do what you could do unaided in
the app, and the import commits without a separate approval step. What still
binds it is what binds you: your seat, your grants, your row scope, and the
scopes you lent when you minted the passport. See
[connect-an-mcp-client.md](connect-an-mcp-client.md).

An installation can require confirmation for `commit_import`; the commit then
stages for a human like any confirm-first call. The assistant cannot upload a
file for you or undo a run, because both need your own signed-in session, so
over MCP the CSV goes across as text in the request.

## Related

- [import-your-linkedin-network.md](import-your-linkedin-network.md): a
  different importer for a different file, landing relationship edges rather
  than companies.
- [connect-an-mcp-client.md](connect-an-mcp-client.md): connecting an assistant.
- [mint-a-passport.md](mint-a-passport.md): issuing the credential it uses.
