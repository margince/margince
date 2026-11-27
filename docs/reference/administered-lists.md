<!-- prose:plain -->
# Lists an admin edits

An admin edits some lists in the app. Each entry has a label, an order, an active
flag and a flag for the built-in entries. The lists come in two shapes,
and what names an entry from outside decides the shape.

| Shape | Use it when | No two entries share | Tables |
|---|---|---|---|
| Keyed | Code, a filter or another table names an entry by a fixed text | `key` | `record_role`, `lead_origin`, `deal_acquisition_source` |
| Label-only | Other tables name an entry by `id`, and nothing names it by a fixed text | The label, compared in lower case with the spaces at its ends removed | `sdr_handoff_reason`, `lead_disqualify_reason` |

A keyed list lets an admin change a label and keep each reference, because the
reference holds the key. `deal.acquisition_source` holds a key of
`deal_acquisition_source`.

A label-only list has no key to compare, so the database holds each label once.
`sdr_handoff_reason` holds it once for each kind the reason applies to.

The columns of each table are in the [entity model](entity-model/README.md).

A new list takes the shape its references need. A list that is named both by `id` and by a fixed
text is keyed. A keyed list is not changed to
label-only, and a label-only list is not given a key, without such a need.
