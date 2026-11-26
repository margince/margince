<!-- prose:plain -->
<!-- describes: frontend/src/features/worklist/worklist.linksuggestions.tsx frontend/src/features/worklist/linksuggestions.queries.ts backend/internal/modules/activities/linksuggestion.go backend/internal/modules/activities/whichdealanswer.go -->
# Suggested links

An extension your admin turned on can suggest which deal, company, contact or project a message belongs to. Margince
itself asks one question here: when several open deals fit an email, it asks you **Which deal is this about?**, see
[Capture](capture.md). This page is for the reader who decides on those suggestions. The Worklist is in
[Your day](your-day.md).

## Where do I find the suggestions?

On **Home**, under the Worklist, there is a **Suggested links** panel. Each row shows the message, the record it names
and who suggested it with their reason: the extension, or Margince. The panel is not shown when there is nothing to
decide.

## What do I do with one?

Nothing is filed on its own.

| Button | What it does |
|---|---|
| **File it there** | Files the message under the record the row names. |
| **Not there** | Declines the suggestion. The message stays where it is, and the same record is not suggested for it again. |

When Margince asked which deal an email is about, each open deal that fits is one row. A deal row shows the deal's
value, if your role lets you see it, and its close date. **File it there** files the email and the rest of its
conversation on that deal, and the other rows of the question leave the panel. **Not there** also tells Margince the
conversation is not about that deal, so it never files it there by itself.

The question also ends when an email of the conversation goes on a deal some other way. That is **Relink**, or Margince
filing it by itself. A deal that closes while it waits leaves the panel. A merged deal is shown as the deal it was
merged into.

## What decides what I see?

- A suggestion shows only when you may decide it: you may change the message, and you may open the record it names.
  The same check runs when you press a button, so a row you see is one you can decide.
- If you may not read messages at all, the panel says that you do not have access to suggested links.
- The contact who wrote a message is never suggested for it.
- A question Margince asked shows only to the one colleague it asked. Nobody else sees it, admins included.
- The panel shows the oldest 50. Decide some and the next ones appear.
