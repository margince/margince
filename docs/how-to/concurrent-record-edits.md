# Editing records while other work is running

The main **Edit** forms for companies, contacts, and deals allow
independent changes to the same record. For example, setting a company's
Customer tier still saves if website research refreshes its logo in the meantime.
No installation setting or database migration is required.

Only the fields changed in the form are sent. A concurrent edit to another
field is preserved. If somebody changes the same field, the save is refused;
close the form, reload the record, review its current value, and edit again.
The refusal does not partially apply the form.

Email, phone, domain, and relationship-type lists are each treated as one
field because their endpoints replace the complete list. Address and social
object members are compared separately, with untouched members preserved.
Values that depend on each other are checked together: a deal's currency and
money figures, partner and attribution, and company and project; a contact's
full name and name parts.

## Shared implementation

[`saveIndependentEdit`](../../frontend/src/screens/independentedit.ts) compares
three readings: the original record captured when Edit opened, the submitted
changes, and the latest server record. The same function serves all three forms,
including custom fields that arrive after the form opens.

The API retains its existing atomic `If-Match` check. A definite
`409 version_skew` triggers a fresh read. Only when all edited fields and their
dependencies still match the original does the UI retry with the fresh version.
The retry remains version-guarded, so a writer landing between that read and
save cannot be overwritten. After three refused attempts, the error remains
visible rather than retrying indefinitely.

Timeouts, permission failures, duplicate-record conflicts, and other errors
are never automatically retried. Fields that become masked are not rebased.
Other API clients retain the existing whole-record version behavior; this is
the main edit forms' shared recovery, not a change to the API's concurrency contract.

The helper's race tests and actual company/contact/deal form tests run in the
standard `make check` frontend suite. They cover separate fields, overlapping
edits, clears, nested objects, replace-sets, monetary dependencies, and another
writer arriving during recovery.
