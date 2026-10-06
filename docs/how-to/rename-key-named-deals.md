# Rename deals still named by their import key

An import that carried no deal title named each deal after the source system's
key, so the pipeline shows `acme-q3-renewal-2` where a user expects a name.
`worker deal-key-names` renames those deals from the source system's export. It
runs once per export, by whoever holds the file, and leaves nothing behind in
the product.

## Prepare the export

A CSV with this exact header, one row per deal to look at:

```
source_system,source_key,source_title
hubspot,acme-q3-renewal-2,Acme Q3 renewal
hubspot,bolt-pilot-7,
```

`source_system` and `source_key` identify the deal as it was imported;
`source_title` is the name the source system shows for it and may be empty. A
spreadsheet's "Save as CSV" is fine, including its byte-order mark.

`source_system` must equal, letter for letter and in the same case, the value
the deal carries in its own `source_system` column — what the importer sent as
`source_system` when it created the deal. The deal read on the API does not
return that column, so list the values from the database the worker uses:

```
SELECT source_system, count(*) FROM deal
 WHERE source_system IS NOT NULL GROUP BY source_system;
```

A deal imported without a `source_system` carries none, and the repair cannot
match it.

The run refuses the whole file, naming the line, when the header differs, a
system or key is empty, a field holds a control character such as a tab, the
same system and key appear twice, or there are no rows.

## Run it

The worker reads its database from `MARGINCE_DSN` like any other worker run.
`--workspace` takes the installation's workspace id, which the API does not
return; read it from that database and pass it exactly:

```
SELECT id, slug FROM workspace WHERE archived_at IS NULL;
```

Dry run first. It writes nothing:

```
worker deal-key-names --workspace <workspace-id> --export deals.csv
```

Read the report, then make it real:

```
worker deal-key-names --workspace <workspace-id> --export deals.csv --apply
```

The report has one row per export row, with the columns `OUTCOME`, `DEAL`,
`SOURCE KEY` and `NEW NAME`, then a count per outcome. A dry run ends with
"Nothing was written."

## Read the outcomes

| Outcome | Meaning | What you do |
| --- | --- | --- |
| `would-rename` | Dry run: one deal matches and would take `NEW NAME`. | Check the name, then run with `--apply`. |
| `renamed` | The deal now carries `NEW NAME`. | Nothing. |
| `no-match` | No live deal of that source is still named by the key. | Nothing, when it was renamed already (by this repair or by a user) or is archived. Otherwise the system or key in the file is not what the deal carries: fix the file. |
| `ambiguous` | More than one live deal shares the key, so none is touched. | Rename them by hand in the app. |
| `no-company` | The export has no title and the deal has no company to name it after. | Rename it by hand in the app. |

A dry run where every row is `no-match` most likely means the `source_system`
value is wrong; check it against the query above first.

## What it will and will not touch

- A deal is renamed only while its name still equals the key. A name a user
  has edited stays.
- Archived deals are untouched.
- The new name is `source_title` when the export has one, otherwise
  `<company> · <stage>`.
- Each rename goes through the ordinary update path: its own audit entry and
  `deal.updated` event, written by `system:deal_key_names`.

Each row is its own transaction. If a run stops partway, the renames before the
failure are committed and the report still prints them; run the same command
again and it finds only what is left.
