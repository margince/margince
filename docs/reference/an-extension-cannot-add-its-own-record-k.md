# An extension cannot add its own record kinds, links, signals or reports

This package of work lets an extension bring its own record kinds, links between records, signals and reports into
Margince. It is for those who build or run extensions. Each feature ships on its own, and the table says which ones
are done.

## How to read this page

| Column | Meaning |
|---|---|
| **Feature** | One piece of the package, in build order. |
| **What someone can do now** | What works today. A planned feature says nothing yet. |
| **Page with the details** | Where the facts are. |
| **State** | **done** is built and merged. **planned** has not started. |

| # | Feature | What someone can do now | Page with the details | State |
|---|---|---|---|---|
| 1 | Audited fix of signal evidence | Correct a signal's type and evidence. The old values stay in the audit log. | [Audited fix of signal evidence](#audited-fix-of-signal-evidence) | done |
| 2 | Company type field | Nothing yet. | None yet | planned |
| 3 | Extension record kinds and link registry | Nothing yet. | None yet | planned |
| 4 | Extension report breakdowns and counts | Nothing yet. | None yet | planned |
| 5 | Extension signal types with structured evidence | Nothing yet. | None yet | planned |
| 6 | Panels and tabs on record pages | Nothing yet. | None yet | planned |
| 7 | "Pick a record" field type | Nothing yet. | None yet | planned |
| 8 | Record page with merged history for extension kinds | Nothing yet. | None yet | planned |
| 9 | Search and lists for extension kinds | Nothing yet. | None yet | planned |
| 10 | Suggested-link resolver | Nothing yet. | None yet | planned |
| 11 | Typed links between core records | Nothing yet. | None yet | planned |
| 12 | Let an unattended extension run land a signal through Ingest | Nothing yet. | None yet | planned |

```mermaid
flowchart LR
    F1[1 Audited fix of signal evidence] --> F5[5 Extension signal types]
    F1 --> F12[12 Land a signal through Ingest]
    F2[2 Company type field] --> F3[3 Record kinds and link registry]
    F3 --> F6[6 Panels and tabs]
    F3 --> F7[7 Pick a record field]
    F3 --> F8[8 Merged history]
    F3 --> F9[9 Search and lists]
    F3 --> F10[10 Suggested-link resolver]
    F3 --> F11[11 Typed links between core records]
    F3 --> F4[4 Report breakdowns and counts]
    F5 --> F12
```

## Audited fix of signal evidence

A signal can be corrected after it is filed. `PATCH /v1/signals/{id}` now accepts `kind` and `evidence` next to
`status`, `note` and `severity`. The request is `UpdateSignalRequest` in `backend/api/crm.yaml`.

| Field | What it does |
|---|---|
| `kind` | Changes the signal's type. Only the six kinds filed by hand are accepted: `stalled_deal`, `champion_left`, `reengagement`, `buying_intent`, `risk` and `other`. |
| `evidence` | Replaces the whole evidence list. |

The edit uses the existing `update` audit action, so no new action exists. The audit row's before image holds the old
kind and evidence, and its after image holds the new ones. An edit that changes nothing writes no audit row.

The old values live only in that audit row. The audit log view shows them to a user who may read it.
An erasure of a contact writes an erase row against each signal about that contact. The history views then hold back
every image written before that row, so the old kind and evidence are no longer served.
The data-access export for a contact does not list signals, so these old values are not in it yet.

The request contract accepts only those six kinds. The store also accepts the signal's current `kind` unchanged, even
when it is outside the six, so a client can send back what it read. Any other change of `kind` is refused with a 422 on
the field `kind`, code `invalid_kind`, and nothing changes. The maintainers decide which of the other signal kinds may be
corrected. Until then they are refused.
