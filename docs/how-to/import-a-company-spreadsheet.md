# Import a spreadsheet of companies

Bring a CSV of companies into the CRM — through an assistant over MCP, or over
REST. This guide is about the decision that actually bites: **what happens to a
row naming a company you already have.**

Every import previews before it writes. The preview counts what the commit will
do, row by row, and nothing lands until you approve the run.

> **Undo covers what a run CREATED, and only that.** A signed-in human can
> reverse a completed CSV run — it archives the rows that run created and that
> nobody has touched since, leaving edited ones alone and naming them. What it
> cannot do is restore a company that existed *before* the run: `update` writes
> the file's values onto it, the old values are gone, and no call puts them
> back. That asymmetry is why `on_duplicate` is worth a minute before you commit
> a hundred rows.

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
refuse an agent principal, so an assistant previews and commits but cannot
upload a file on your behalf or reverse a run afterwards.

## Step 1 — Map your columns

The importer does not guess. Name each source column and the field it feeds:

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

## Step 2 — Decide what to do about duplicates

This is the choice worth making on purpose. `on_duplicate` takes three values.

| Value | A row naming a company already here | Use it when |
|---|---|---|
| `create` *(default)* | Lands a second record and files the pair for review | One company typed by a human |
| `skip` | Leaves the existing record untouched | You want only the genuinely new rows |
| `update` | Writes the row's values onto the existing record | The file is the better copy |

`create` is the default for compatibility, and it is usually the wrong choice
for a spreadsheet: a hundred rows of companies you already have becomes a
hundred twins, each needing a merge.

**`update` is the correction case** — an export you have cleaned up, where the
file should win. It overwrites only when your row names **exactly one** company,
matching its name outright — legal form included, and on the same axis (a trading
name against a trading name, a registered name against a registered one).
Anything short of that is reported as a skip, and left for you.

That bar is deliberately higher than the one the CRM uses to *suggest* merges.
Suggesting costs a glance; overwriting cannot be undone onto a company the run
did not create. So:

| Your row | Already in the CRM | What happens |
|---|---|---|
| `Kestrel Data GmbH` | `kestrel data gmbh` | **Updated** — same name, different casing |
| `Straße Co` | `STRASSE Co` | **Updated** — ß and ss are the same letter |
| `Acme Inc` | `Acme GmbH` | **Skipped** — different legal entities |
| `Kestrel Data Systems` | `Kestrel Data Solutions` | **Skipped** — similar, not the same |
| `Acme` | `Acme Inc` | **Skipped** — the suffix is part of the name |
| `Kestrel Data` | *two companies both called* `Kestrel Data` | **Skipped** — your file did not say which |

That last row is the one worth knowing about: **two companies may legitimately
share a name**, the CRM does not stop it, and no amount of matching can tell which
one a row means. The run refuses rather than picking.

Near misses are reported as skips naming what to do about them, so nothing goes
by unseen — the decision simply stays yours rather than being made by an importer
that could not tell two companies apart.

It also writes only where you could write anyway. A company you are not
permitted to see is neither mentioned in the report nor edited — an import
cannot become a blind edit of a colleague's private record.

## Step 3 — Read the report before you commit

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

Two things to know about those numbers:

- **`created`, `updated`, `unchanged` and `skipped` sum to `rows_read`.** Every
  row has exactly one outcome. If they do not add up, the report is hiding
  something.
- **`duplicates` sits outside that sum.** It is not a fifth outcome — it counts
  rows already counted elsewhere. It is the number you actually weigh: *"100
  companies, 94 of them already here"* is a different decision from *"100 new
  companies"*, and the two are indistinguishable from `created` alone.
- **A company you are not permitted to see is not counted or named.** Row scope
  applies to the report as much as to the record, so a preview never confirms the
  existence of a colleague's private capture. Such a row previews as a create and
  is left alone on commit, which is the only safe direction for the two to
  differ.

`unchanged` means matched and nothing differs. It is separate from `updated` so
that re-running a file you already applied reports no work rather than a hundred
writes and an audit trail to match.

**`issues` names any row that will not land**, in terms of the file rather than
the database — an unusable `size_band`, a name the importer will not overwrite on
a guess. Fix those in the source and preview again. (Rows carry a `line` where the
importer could derive one; a file identified by its own key column reports `0`.)

## Step 4 — Commit

Approve the run by its id. The commit is checkpointed and idempotent on the
source key, so a re-run of the same file converges rather than duplicating, and
a run interrupted midway resumes from where it stopped rather than starting
over.

Ask afterwards, or read the report again: the same shape reports what the run
*did* once it has finished.

## Asking an assistant to do it

Over MCP this is one instruction:

> Import this CSV as organizations. Tell me how many are new and how many are
> already here before you commit anything, and update the existing ones rather
> than creating duplicates.

The assistant runs on your passport, so it can do what you could do unaided in
the app, and the import commits without a separate approval step. What still
binds it is what binds you: your seat, your grants, your row scope, and the
scopes you lent when you minted the passport. See
[connect-an-mcp-client.md](connect-an-mcp-client.md).

Two caveats. An installation can require confirmation for `commit_import`, in
which case the commit stages for a human like any confirm-first call. And the
assistant cannot upload a file for you or undo a run — both need your own
signed-in session — so over MCP the CSV goes across as text in the request.

**Ask for the counts before the commit.** An assistant that previews, reports and
waits is using the feature as designed; one that previews and commits in the same
breath has skipped the step the preview exists for.

## Related

- [import-your-linkedin-network.md](import-your-linkedin-network.md) — a
  different importer for a different file, landing relationship edges rather
  than companies.
- [connect-an-mcp-client.md](connect-an-mcp-client.md) — connecting an assistant.
- [mint-a-passport.md](mint-a-passport.md) — issuing the credential it uses.
