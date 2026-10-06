# Rename deals still named by their import key

An import that carried no deal title named each deal after the source system's
key, so the pipeline shows `acme-q3-renewal-2` where a person expects a name.
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

The run refuses the whole file, naming the line, when the header differs, a
system or key is empty, the same system and key appear twice, or there are no
rows.

## Run it

The worker reads its database from `MARGINCE_DSN` like any other worker run.
Take the workspace id from the installation's admin.

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
| `no-match` | No live deal of that source is still named by the key. | Nothing; it was renamed already, or is archived. |
| `ambiguous` | More than one live deal shares the key, so none is touched. | Rename them by hand in the app. |
| `no-company` | The export has no title and the deal has no company to name it after. | Rename it by hand in the app. |

## What it will and will not touch

- A deal is renamed only while its name still equals the key. A name a person
  has edited stays.
- Archived deals are untouched.
- The new name is `source_title` when the export has one, otherwise
  `<company> · <stage>`.
- Each rename goes through the ordinary update path: its own audit entry and
  `deal.updated` event, written by `system:deal_key_names`.

Each row is its own transaction. If a run stops partway, the renames before the
failure are committed and the report still prints them; run the same command
again and it finds only what is left.
