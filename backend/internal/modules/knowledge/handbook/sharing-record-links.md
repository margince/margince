<!-- prose:plain -->
<!-- describes: backend/internal/compose/publicrecordpreview.go backend/internal/compose/recordpreviewdata.go frontend/src/lib/recordaddress.ts -->
# Sharing record links

Paste a deal, company or contact page's address into a message to share it.
The preview names the record and shows a short line of context.

A deal's card shows its company and stage. A contact's card shows the job title
and employer. A company's card shows its main domain when one is set.

Anyone with the address can read those card fields without signing in.
Notes, activity, files, amounts, email addresses and phone numbers stay private.
Opening the full record needs the reader's own sign-in and record access.

An old link still opens the same record. The browser then updates its address
so a preview service can read which record it names. The service may keep an
older card for a while; fetching the updated address reads the current card.

An archived record has no public card. Each messaging app chooses how it shows
a preview and how long it keeps it.
