# Change mail capture from an extension

An extension can change two answers core gives while it files a captured mail: which contacts and companies the mail names, and which deal it belongs to. This page is for an extension author. The types are in `backend/pkg/extension/mailhooks.go`.

## Register the hooks

Set `MailHooks` on your `Extension`. Both functions are optional.

```go
MailHooks: extension.MailHooks{
    Parties: func(ctx context.Context, mail extension.MailView, p extension.MailParties) (extension.MailParties, error) { … },
    Deal:    func(ctx context.Context, mail extension.MailView, d extension.MailDeal) (extension.MailDeal, error) { … },
    Reasons: []string{"po_number_match", "credit_hold"},
},
```

| Hook | When it runs | What you may return |
|---|---|---|
| `Parties` | After core derived the mail's contacts and companies, before the deal rule reads them. | The same lists, or lists of live contacts and at most two live companies, all visible to the mailbox owner. Core never counts its own company. |
| `Deal` | At the end of the deal rule. | `none`, a deal to link, or an `ambiguous` answer with fewer candidates. |

## Declare your reasons

`Reasons` lists the keys your answers may give. A key is lower snake case, at most 40 characters, and a unit may declare up to 16. A unit with a hook declares at least one. Core refuses a bad list when it starts.

Set `Reason` to one of your keys whenever you change something. Leave it empty when you change nothing. A change that names a key you did not declare is ignored, and the capture trace says so.

Ship a label for each key in your own translations, in every language you ship. The key is `ext<Unit>.captureReason.<key>`. For `deal-rules` and `po_number_match` that is `extDealRules.captureReason.po_number_match`. Settings > Capture shows the label next to the unit's name. Where a label is missing it shows the key.

## What a hook may read

`MailView` holds the mail's id, whether it is a meeting, its subject, its participants' addresses, and the start of its text (`Body`, at most 8,000 characters). It has no files. It runs only for a mail the deciding seat may read, so a held mail's text never reaches a hook.

`MailView.Lookup` reads one row of your own tables. It takes the statement and its arguments as `Tx.QueryRow` does, with your table spelled `ext.ext_<unit>_<table>`:

```go
var deal string
err := mail.Lookup(`SELECT deal::text FROM ext.ext_deal_rules_ref WHERE $1 LIKE '%' || ref || '%'`, mail.Body).Scan(&deal)
```

A lookup cannot write. It runs in its own read-only transaction as the unit role, so core's tables stay out of reach, and it ends after 250 milliseconds. A hook gets a context and no `Runtime`, so it can read nothing else. The context ends after 250 milliseconds, so a hook that waits should watch it.

## What core promises

- Hooks run in unit-name order. Each sees the previous result.
- A hook must give the same answer for the same input. Core asks again whenever it re-decides a mail.
- An error, a panic, or a call that runs longer than 250 milliseconds is a failure. The hook decides nothing, and neither does core. A mail that already has a link keeps it. A mail with none stays unfiled and is tried again later. Later hooks do not run for that mail.
- A hook cannot change what a colleague decided. A mail that is already filed never reaches a hook. A deal a colleague said the mail is "Not about" is dropped from the answer, and core files by its own rule instead.
- A deal you link is filed without the model's topic check, because you decided it by your own rules.
- Core records the change. The deal link's audit row names each unit, the hook and its reason under `extension_hooks`.
- Settings > Capture shows who filed each mail: core with the rule it used, or your unit with your reason. It also shows when your hook answered `none`, took a link away, failed, or was ignored, and when a later repair moved or took back a link.

An answer core cannot use is dropped as a whole. That covers an id that is not a live record, a third company, a candidate the rule did not offer, and an `ambiguous` answer where core found none. It also covers a contact, company or deal the mailbox owner cannot see. The trace files such an answer as ignored.
