# Change mail capture from an extension

An extension can change two answers core gives while it files a captured mail: which contacts and companies the mail names, and which deal it belongs to. This page is for an extension author. The types are in `backend/pkg/extension/mailhooks.go`.

## Register the hooks

Set `MailHooks` on your `Extension`. Both functions are optional.

```go
MailHooks: extension.MailHooks{
    Parties: func(mail extension.MailView, p extension.MailParties) (extension.MailParties, error) { … },
    Deal:    func(mail extension.MailView, d extension.MailDeal) (extension.MailDeal, error) { … },
},
```

| Hook | When it runs | What you may return |
|---|---|---|
| `Parties` | After core derived the mail's contacts and companies, before the deal rule reads them. | The same lists, or lists of live contacts and at most two live companies. Core never counts its own company. |
| `Deal` | At the end of the deal rule. | `none`, a deal to link, or an `ambiguous` answer with fewer candidates. |

Set `Reason` to one sentence whenever you change something. Leave it empty when you change nothing.

## What a hook may read

`MailView` holds the mail's id, whether it is a meeting, its subject, and its participants' addresses. It has no body and no files. A hook gets no context and no `Runtime`, so it can read and write nothing else. It runs only for a mail the deciding seat may read.

## What core promises

- Hooks run in unit-name order. Each sees the previous result.
- A hook must give the same answer for the same input. Core asks again whenever it re-decides a mail.
- An error, a panic, or a call that runs longer than 250 milliseconds leaves the previous result as it was.
- A hook cannot change what a colleague decided. A mail that is already filed never reaches a hook. A deal a colleague said the mail is "Not about" is dropped from the answer, and core files by its own rule instead.
- A deal you link is filed without the model's topic check, because you decided it by your own rules.
- Core records the change. The deal link's audit row names each unit, the hook and its reason under `extension_hooks`.

An answer core cannot use is dropped as a whole. That covers an id that is not a live record, a third company, a candidate the rule did not offer, and an `ambiguous` answer where core found none.
