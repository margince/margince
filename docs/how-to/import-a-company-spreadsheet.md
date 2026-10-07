<!-- prose:plain -->
# Import a spreadsheet of companies

Put a CSV of companies into the CRM through an assistant over MCP, or over REST. A run can **add**
companies you do not have, and **correct** ones you do.

Every import shows a preview before it writes. The preview counts what the commit will do, row by row, and
nothing lands until you approve the run.

> **Undo covers only what a run created.** A human who has signed in can undo a CSV run that is done. It
> archives the rows that run created and that no one has touched since. It does not touch edited rows, and
> names them. It cannot get back a company that existed *before* the run: a correction writes over the
> old values, and no call puts them back.

## The shape of a run

The steps are the same over MCP and REST:

1. **Preview**: hand over the file and a map of columns. This writes nothing, and returns a `run_id` and a report.
2. **Read the report**: the counts, and the issues that name any row that will not land.
3. **Commit**: approve the run by its ID. *This* is the call that writes.

Over MCP that is three tool calls: `preview_import` (the CSV goes in the call, as text),
`read_import_report` and `commit_import`.

Over REST you upload the file first, so it is four calls:

- `POST /v1/imports/sources`, with the file as `multipart`;
- `POST /v1/imports`, which names the `source_ref` that the first call returns;
- `GET /v1/imports/{id}/report`;
- `POST /v1/imports/{id}/approve`.

**Only a human can upload or undo.** Both need a session where a human has signed in, and refuse an agent.
So an assistant can preview and commit, but it cannot upload a file for you, or undo a run after it.

## Adding companies

Name each source column, and the field it fills. The importer does not guess:

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

A company can take `display_name`, `legal_name`, `industry`, `size_band`, `description`, `domain`,
`author`, and the 6 address fields: `address.line1`, `address.line2`, `address.city`, `address.region`,
`address.postal_code`, `address.country`. Map a name it does not take, and the run is refused before
anything is written. The answer lists what it does take.

`author` is who created the row in the system the file is from. Margince keeps the value as written. If
the value is the email of a Margince user, the record also links to that user, so it shows their current
name. `author` is set when the import creates the record. A later file does not change it.

**A file of contacts takes `lead` or `contact`.** Pick by its source:

- A list from a machine takes `lead`. That is a list pulled from web pages, a list you paid for, or a
  list of contacts scanned at an event. Its rows land with no work on them yet, and someone turns the good
  ones into contacts.
- A file of contacts the business already knows takes `contact`. That is a move off another CRM, or a
  customer list from a system you no longer use. Those rows were checked in that other system, so they
  land as contacts, through the same checks for copies.

### What happens to a company you already have

`on_duplicate` decides. It takes `create` or `skip`. `create` is the default: it lands a second record,
and files the pair for review. `skip` leaves the record you have as it is.

For a spreadsheet, `create` is most often the wrong pick. 100 rows of companies you already have become
100 copies, and each one needs a merge. The preview tells you how many before you commit; see
`duplicates` below.

**Neither of these corrects anything.** For that, the file has to say *which* company each row is.

## Correcting companies

Give each row the ID of the company it is, and map that column to **`id`**:

```
id,display_name,city
01a02ed1-0866-7567-b567-2abcf76e5c1e,Kestrel Data,Bremen
```

A row with an `id` **updates that company**. The ID names one record, so there is no match and no guess.
Read the companies out first to get their IDs, edit the file, and import it back.

Rules for the `id` column:

- **An empty `id` creates a company, as usual.** One file can carry corrections and new companies
  together, as long as some other column names every row. A file whose *only* naming column is `id` needs
  one on every row. A row that nothing names cannot be imported again, and no one can undo it; see `source_key` below.
- **Margince refuses an ID that matches no record**, and reports the row as a skip that says so. It never
  creates the row under a new ID. So a stale export, or a typing error, sends you back to the file, and
  leaves no record you did not expect.
- **Nothing is written to `id` itself.** It names the record, and is not a value the record holds.

### Which column names a row

Every row needs one column that names it *within your file*. That column makes an import of the same file
update rows, and not copy them. It also lets an undo find what a run created. The importer uses the
company name by default. `source_key` names a different column when you want another one.

Two shapes work:

- **A file of corrections only.** `id,city` is enough. The ID names the row and the record, and every
  row must have one.
- **A file of both.** Map a column every row has (the company name will do), and leave the `id` column
  empty on the rows that are new.

Margince reports a row with no naming value at all as a line it cannot use, and does not import it. So
nothing lands that no one could find again later.

### Why not match on the name?

A name does not tell you which company it is. The matcher that finds *likely* copies answers one question:
should a human look at this pair? To answer it, the matcher makes names less exact:

- It drops the legal form, so `Acme Inc` and `Acme GmbH` are the same text. The CRM sends those to a
  human to review, because they can be different companies.
- It scores a trade name against a registered one. So `Kestrel Data` in your row matches a company
  registered under that name, but that trades as something else.
- Two companies may share a name, and that is allowed. Nothing stops it, and where several match, the
  matcher picks one, with no rule for which.

Each of those does no harm when the result is "show a human two records". When the result decides a write,
each one can write over the wrong company, and you cannot undo that. An ID fails in none of these ways.

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

- **These counts add up to `rows_read`**: `created`, `updated`, `unchanged` and `skipped`. Every row has one
  result. If they do not add up, the report is missing something.
- **`duplicates` is not part of that count.** It counts rows that are already counted under another
  result. It is the number to look at. "100 companies, 94 of them already here" is a different decision
  from "100 new companies". `created` by itself cannot tell them from each other.
- **`unchanged` means matched, and nothing is different.** It is separate from `updated`, so that a file
  you already applied, when you run it again, reports no work. It does not report 100 writes, and an
  audit log to match.
- **A company you cannot see changes nothing.** It is not counted or named, and does not change the result.

More on that last point: Margince creates your row as it would if no such company existed. A company you
*can* see is still reported, even when a company you cannot see is a closer match. To skip on that one
would tell you that it exists, and that is a fact about the private record of someone else. The cost is a copy, which
the review queue picks up. A skip would tell someone a private fact, and no merge undoes that.

**`issues` names any row that will not land**, in the words of the file, not the database. Examples are a
`size_band` it cannot use, or an ID that matches no record. Fix those in the source, and preview again.

## Committing

Approve the run by its ID. The commit saves its place as it goes, and it is idempotent on the source key.
So a second run of the same file ends in the same state, and does not make copies. A run that stops in the
middle starts again from where it stopped, and not from the start.

A correction is not recorded as something the run created. So `undo` archives only the companies a run
added, and never one that was already there and was edited.

The report keeps its shape after the commit: the same fields then report what the run *did*.

## Asking an assistant to do it

Over MCP this is one request:

> Import this CSV as companies. Tell me how many are new and how many are
> already here before you commit anything.

Or, for a file of corrections:

> Read out our companies with their IDs, then import this CSV. Each row carries
> the ID of the company it updates.

The assistant runs on your passport. So it can do what you could do on your own in the app, and the import
commits without a separate approval step. The limits on you are the limits on it. They are your seat, your
grants, your row scope, and the scopes you gave when you created the passport. See
[connect-an-mcp-client.md](connect-an-mcp-client.md).

An installation can require a confirm for `commit_import`. The commit then waits for a human, like any call
that needs a confirm first. The assistant cannot upload a file for you or undo a run, because both need
your own session. So over MCP the CSV goes across as text in the request.

## See also

- [import-your-linkedin-network.md](import-your-linkedin-network.md): a different importer for a different
  file, which lands links to contacts, not companies.
- [connect-an-mcp-client.md](connect-an-mcp-client.md): how to connect an assistant.
- [mint-a-passport.md](mint-a-passport.md): how to issue the credential it uses.
