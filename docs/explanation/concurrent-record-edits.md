<!-- prose:plain -->
# Editing records while other work is running

Editing in the **Details** panel of a company, contact, deal or lead writes only
the fields a user changed. So two separate edits to one record both land. What
a user sees, including the message when a colleague wrote the same field first,
is in the handbook, [records.md](../handbook/records.md). Below is how the edits
merge.

Email, phone, domain, and relationship type lists each count as one field,
because their endpoints take the whole list and keep only that. The parts of an
`address` and a `social` object are checked one by one, and parts nobody touched
are kept. Values that depend on each other are checked together. On a deal that
is the currency and the money values, partner and attribution, and company and
project. On a contact it is the full name and the name parts.

## One shared function

[`saveIndependentEdit`](../../frontend/src/screens/independentedit.ts) checks
three versions of a record. The first is the original, read when editing
started. The second is the changes the user sent, and the third is the newest
record on the server. The same function serves all four record types, including
their separate custom-field sections.

The API keeps its `If-Match` check, which passes or fails as one step. A clear
`409 version_skew` answer starts a new read. The UI tries again with the new
version only when every edited field, and every value it depends on, still
matches the original. The second try still carries a version check, so a writer
that lands between that read and the write cannot be written over. After three
refused tries, the error stays on screen and the panel stops trying.

A timed-out call, a permission failure, a duplicate-record conflict, or any other error
is never tried again on its own. If a field the edit touches, or one it depends on,
becomes masked, the whole edit is refused. Other API clients keep the whole-record version check. This
retry lives only in the Details panel, and the contract of the API for two writers
at once does not change.

The tests of the function for two writers at once run with the frontend tests
in `make check`. So do real form tests for companies, contacts and deals.
They cover separate fields, and edits that touch the same field. They also cover
clears, objects inside objects, lists sent whole, values that depend on money,
and another writer that lands while the retry runs.
