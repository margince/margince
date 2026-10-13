<!-- prose:plain -->
# Import a spreadsheet or a ZIP over MCP or REST

This page is for a developer or an assistant that imports a CSV of companies, or a ZIP of CSV files,
through MCP or REST. A user who imports in the app reads the handbook: the
[Settings](../handbook/settings.md) page covers **Data import**, its test run and how to correct
companies. Undo is in
[What is kept, what is destroyed](../handbook/retention-exports-and-deletion.md).

A run can **add** companies you do not have, and **correct** ones you do. Every run shows a preview
before it writes, and nothing lands until the run is approved.

## The shape of a run

The steps are the same over MCP and REST:

1. **Preview**: hand over the file and a map of columns. This writes nothing. It returns a `run_id` at
   once, and the worker then tests the file against your records.
2. **Read the report** once the run is `awaiting_approval`. It holds the counts and the issues.
   An issue names a row that will not land. A test that stops returns the run to `pending`, with the
   reason in `error`.
3. **Commit**: approve the run by its ID. *This* is the call that writes.

Over MCP that is three tool calls: `preview_import` (the CSV goes in the call, as text),
`read_import` with `view: report`, and `commit_import`. `preview_import` waits up to 20 seconds for
the test. If the test takes longer, it answers `state: validating` with a `next` line; read
`read_import` again until `state` is `awaiting_approval`. A test that refuses the file answers the
refusal, and the run goes back to `pending`.

One assistant tests one run at a time. A second `preview_import` while its first test still runs is
refused, and the refusal names the run still being tested. `commit_import` answers at once while the run
goes on, so read `read_import` with `view: report` again until `state` is `complete`. On a run that
`failed`, `commit_import` resumes the write.

Over REST you upload the file first, so it is five calls:

- `POST /v1/imports/sources`, with the file as `multipart`;
- `POST /v1/imports`, which names the `source_ref` that the first call returns and answers at once
  with the run `validating`;
- `GET /v1/imports/{id}`, read again until `status` is `awaiting_approval`, then
  `GET /v1/imports/{id}/report`;
- `POST /v1/imports/{id}/approve`, which answers at once while the run goes on;
- `GET /v1/imports/{id}`, read again until `status` is `complete`.

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

**A file of contacts takes `lead` or `contact`.** A list from a machine (pulled from web pages, paid for,
or scanned at an event) takes `lead`. A file of contacts the business already knows, such as a move off
another CRM, takes `contact`.

### What happens to a company you already have

`on_duplicate` decides. It takes `create` or `skip`. `create` is the default, and the one the app uses:
it lands a second record, and files the pair for review. `skip` leaves the record you have as it is.

For a spreadsheet, `create` is most often the wrong pick. 100 rows of companies you already have become
100 copies, and each one needs a merge. The report's `duplicates` count tells you how many before you
commit.

**Neither of these corrects anything.** For that, the file has to say *which* company each row is.

## Correcting companies

Give each row the ID of the company it is, and map that column to **`id`**:

```
id,display_name,city
01a02ed1-0866-7567-b567-2abcf76e5c1e,Kestrel Data,Bremen
```

A row with an `id` **updates that company**. Read the companies out first to get their IDs, edit the
file, and import it back. An empty `id` creates a company. An ID that matches no record is reported as a
skip, never created under a new ID. Nothing is written to `id` itself.

### Which column names a row

Every row needs one column that names it *within your file*. That column makes an import of the same file
update rows, not copy them, and lets an undo find what a run created. The importer uses the company name
by default; `source_key` names a different column. The report's `source_key_used` says which one it used.

Over the API, a file of corrections only (`id,city`) is enough: the ID names every row. The app's
**Data import** screen always asks for the name column, so there a file of corrections carries both.

### Why not match on the name?

The matcher that finds *likely* copies answers one question: should a human look at this pair? To answer
it, it makes names less exact. It drops the legal form, and scores a trade name against a registered one.
Where several companies share a name, it picks one, with no rule for which.

Each of those does no harm when the result is a review. When the result decides a write, each one can write over the wrong company, and no
call undoes that. An ID fails in none of these ways.

## Importing a ZIP over REST

A ZIP holds one CSV for each record type. The file name gives the type: `companies.csv`,
`contacts.csv` or `leads.csv`. A run lands companies first, then contacts, then leads, so a contact can
link to a company in the same ZIP.

1. Read `GET /v1/imports/format`. It lists each file, its columns and what goes in each one, and the
   order the files land in. `GET /v1/imports/template` returns the same as a ZIP to fill in.
2. Upload the ZIP to `POST /v1/imports/sources`, with no `object`.
3. Read `parts` in the answer: one profile per file, with a map for each type. `ignored` names the
   files no type takes.
4. Call `POST /v1/imports` with `parts`. Each part names its `file`, its `object` and its `mapping`.
5. Read the report once the run is `awaiting_approval`, then approve the run as for one CSV.

A ZIP holds at most 20 files, and the upload limit applies to the ZIP. Two files of one type are
refused.

A contact's `company_ref` names its company by the company's `ref` in this upload or an earlier one, its
domain, its name or its ID. `on_broken_link` decides a row whose link finds nothing: `skip_row` leaves
the row out, and `drop_link` lands it without the link. Both are listed in the report.

`source_name` files the run's records under one name, so the same export uploaded again updates them.
The upload's answer holds one in `suggested_source_name`.

`GET /v1/imports/{id}/problems.csv` returns the report's issues as a spreadsheet, one row per issue.
The template and this spreadsheet need a session where a human has signed in, as the upload does. A
passport may read the format.

## Importing several files over MCP

`preview_import` takes `files` in place of `object`, `csv` and `mapping`. Each file names its `name`, its
`object`, its `csv` text and, if you want, its own `mapping`. The run takes one file per type, and the
CSV text of all the files together is at most 1,000,000 bytes. `source_name` and `on_broken_link` work
as they do over REST.

`read_import` with `view: format` answers what `GET /v1/imports/format` answers. The answer to
`preview_import` lists each file under `files`, with the map it settled on and the columns no field
takes. `part` is the name the report uses for the file, which is its type's template name.

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

`created`, `updated`, `unchanged` and `skipped` add up to `rows_read`. `duplicates` is not part of that
count: it counts rows already counted under another result, and it is the number to read before a commit.
The app's preview shows the four, not `duplicates`.

A company the caller cannot see is not counted or named. Margince creates the row as if no such company
existed, because a skip would tell the caller that a private record exists. The cost is a copy, which the
review queue picks up. No merge takes back a private fact once it is told.

## Committing

Approve the run by its ID. The answer comes at once, with the run `running`, and the run goes on after it;
read the run until its `status` is `complete`. The commit saves its place as it goes, and it is
idempotent on the source key.
A run that stops in the middle starts again from where it stopped. The report keeps its shape after the
commit: the same fields then report what the run *did*.

## Asking an assistant to do it

Over MCP this is one request:

> Import this CSV as companies. Tell me how many are new and how many are
> already here before you commit anything.

Or, for a file of corrections:

> Read out our companies with their IDs, then import this CSV. Each row carries
> the ID of the company it updates.

The assistant runs on your passport. The limits on you are the limits on it. They are your seat, your grants,
your row scope, and the scopes you gave when you created the passport. See
[connect-an-mcp-client.md](connect-an-mcp-client.md).

An installation can require a confirm for `commit_import`. The commit then waits for a human, like any call
that needs a confirm first.

## See also

- [The LinkedIn import](../explanation/relationship-graph.md#the-second-tier-an-imported-linkedin-network):
  a different importer for a different file. It stores your connections and links the confirmed matches
  to contacts, not companies.
- [connect-an-mcp-client.md](connect-an-mcp-client.md): how to connect an assistant.
- [mint-a-passport.md](mint-a-passport.md): how to issue the credential it uses.
