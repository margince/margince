<!-- prose:plain -->
# Rename deals still named by their import key

When an import gave a deal no title, it named the deal after the key in the source
system. So the pipeline shows `acme-q3-renewal-2` where a user expects a name.
`worker deal-key-names` renames these deals from an export of the source system. The
user who holds the file runs it once for each export, and it leaves nothing behind in
the product.

## Make the export

Make a CSV with this exact header, and one row for each deal to look at:

```
source_system,source_key,source_title
hubspot,acme-q3-renewal-2,Acme Q3 renewal
hubspot,bolt-pilot-7,
```

`source_system` and `source_key` pick out the deal as the import made it.
`source_title` is the name the source system shows for it, and it may be empty. You can use
"Save as CSV" in a spreadsheet, and the byte order mark it adds is no problem.

`source_system` must match the value in the deal's own `source_system` column letter for letter, in the
same case. That is the value the import gave as `source_system` when it made the deal. The API does
not return that column when you read a deal, so list the values from the database the worker uses:

```
SELECT source_system, count(*) FROM deal
 WHERE source_system IS NOT NULL GROUP BY source_system;
```

A deal that the import made without a `source_system` has no value there, and the repair cannot
match it.

The run refuses the whole file, and names the line, in these cases. The header is wrong, a
system or key is empty, or the same system and key show up twice. It also
refuses a field that is not UTF-8 or that holds a control character such as a tab,
and a file with no rows.

## Run it

The worker reads its database from `MARGINCE_DSN`, like any other worker run.
`--workspace` takes the workspace id of the installation, and the API does not
return it. Read it from that database and pass it as it is:

```
SELECT id, slug FROM workspace WHERE archived_at IS NULL;
```

Do a dry run first. It writes nothing:

```
worker deal-key-names --workspace <workspace-id> --export deals.csv
```

Read the report, then make the change real:

```
worker deal-key-names --workspace <workspace-id> --export deals.csv --apply
```

The report has one row for each export row, with the columns `OUTCOME`, `DEAL`,
`SOURCE KEY` and `NEW NAME`, then a count for each outcome. A dry run ends with
`Nothing was written.`

## Read the outcomes

| Outcome | Meaning | What you do |
| --- | --- | --- |
| `would-rename` | Dry run: one deal matches, and it would take `NEW NAME`. | Check the name, then run with `--apply`. |
| `renamed` | The deal now has `NEW NAME`. | Nothing. |
| `no-match` | No live deal from that source still has the key as its name. | Nothing, when someone renamed it already (this repair or a user), or it is in the archive. If not, the system or key in the file is not what the deal has: fix the file. |
| `ambiguous` | More than one live deal has the key, so the run changes no deal. | Rename them by hand in the app. |
| `no-company` | The export and the deal give no name other than the key. The title is empty or the same as the key, and the deal has no company, or `<company> · <stage>` is the key itself. | Rename it by hand in the app. |

When every row of a dry run is `no-match`, the `source_system` value is most likely
wrong. Check it against the query above first.

## What it will and will not touch

- The run renames a deal only while its name is still the key. A name a user
  changed stays.
- The run does not touch a deal in the archive.
- The new name is `source_title` when the export has one that is not
  the key. If not, it is `<company> · <stage>`. The name is on one line: tabs become
  spaces, and the run drops other control characters and characters that change which way text reads.
  It also makes each run of spaces one space.
- Each rename takes the normal update path, with its own audit entry and
  `deal.updated` event, made by `system:deal_key_names`.

Each row is its own transaction. If a run stops part of the way, the renames before the
error are saved, and the report still prints them. Run the same command
again, and it finds only the rows still to rename.
