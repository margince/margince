<!-- prose:plain -->
# Lists an admin edits

An admin edits five lists in the app. Each entry has a label, an order, an active
flag and a flag that protects the built-in entries. The lists come in two shapes,
and what refers to an entry decides the shape.

| Shape | Use it when | An entry is unique by | Tables |
|---|---|---|---|
| Keyed | Code, a filter or another table refers to an entry by a fixed text | `key` | `record_role`, `lead_source`, `deal_acquisition_source` |
| Label-only | Other tables refer to an entry by `id` | The label, trimmed and with case folded | `sdr_handoff_reason`, `lead_disqualify_reason` |

A keyed list lets an admin change a label and keep each reference, because the
reference holds the key. `deal.acquisition_source` holds a key of
`deal_acquisition_source`.

A label-only list has no key to compare, so the database holds each label once.
`sdr_handoff_reason` holds it once for each kind the reason applies to.

A new list takes the shape its references need. A keyed list is not changed to
label-only, and a label-only list is not given a key, without such a need.
