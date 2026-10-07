<!-- prose:plain -->
# The record nouns

One rule covers both record nouns: the reader and the program say the same
word, and the schema follows.

| The record | What all code and text calls it | Dropped words |
|---|---|---|
| A human an installation does business with | `contact` | `person`, `people`, `persons` |
| A company an installation does business with | `company` | `organization`, `org`, `account` |

"All code and text" is the whole stack. Over the network that is the URL, the
HTTP path, the `operationId` and the public event type. It is also the
`traverse` name in the search DSL and the `record_type` input of the agent tool.
In the tree it is every table, column and other schema name. It is also
the Go name, the file name, the `i18n` key and the text in `docs/`.

## Why one word in every place

One word per record on the screen, in the API and in the schema means no one
has to map one word to another. A split costs something in every place:

- A reader who searches for the contact table finds nothing and thinks there is
  none.
- A model that asks for `record_type: "company"` is refused by a register that
  takes only `"organization"`.
- The browser says `#/contacts/:id` while the request it sends says
  `GET /people/{id}`, so every new screen has to learn they are the same thing.
- A third spelling comes in with no one deciding on it: the `traverse` name in
  the search DSL followed the kind word and read `persons`.

Both nouns follow the same rule, and the word is the one on the screen. A
program is easier to read when it agrees with the screen.

## The words that are not these records

These words are not the record nouns. Do not rename them.

**Not a contact.** `personal`: a counterparty verdict, a mail posture, a purge
scope, the container of a `capture_exclusion`. `in_person`: the kind of a
consent event, where the word means the humans are in the same place.
`personality_md`: the text of a voice profile. `personnel`: a level of private
data. `persona`, `impersonate`, `salesperson`.

In German, `Personen` and `personenbezogene Daten` are the legal words and are
not this record. **`Settings → People`** is the seats group, next to Company and
Sales. It holds the pages about members with a license. It is the one screen
where the word still means the humans who work here.

**Not a company.** A meeting `organizer`, the word `organizational` and a
`.org` domain. The `organizations` name in Microsoft sign-in, a fixed part
of the path at `login.microsoftonline.com`: renaming it stops sign-in working.
The `ORG` field of a vCard, and the type names of `schema.org`.

**Spelled `contact` but not this record.** `graph_contact_edge` links two
contacts on one thread, and with `contact_a` and `contact_b` it reads as
what it is. Also `organization_fact.field = 'contact_email'`,
`partner.last_contact_at`, `intro_request.route_type = 'through_contact'`, and
a lead whose status is `contacted`.

## What holds it

Three scans, each taking its list of files from the tree:

- `frontend/src/i18n/record-noun.test.ts` checks the copy both ways.
  Every key that offers the record type carries the contact noun. No catalog
  value says the dropped noun, unless its key is listed there as one where the
  word means a human. That list is grouped by the reason each key stays.
- `backend/gates/contactvocabulary_test.go` checks the code. It fails on the
  dropped word in the path or text of every file git tracks. It takes the
  sentences where the word means a human from the catalogs above, and does not
  keep a second copy. The other record noun has a scan of the same shape.
- `backend/internal/shared/gatekit/migrationrenames.go` is not a gate itself. A
  scan that reads migration text sees the first name of a table or column. So
  every scan reads through the `ALTER ... RENAME` statements the migrations
  declare. Without it the other scans miss every name that changed.

Audit rows keep the dropped word for good. `trg_audit_no_mutate` refuses an
UPDATE on `audit_log`, which is what makes the audit log safe to trust. So a
change recorded before a rename still says `entity_type = 'person'` or
`'organization'`. The two read paths that filter the audit log by record type accept
both words, and `backend/internal/compose/auditlegacytype.go` holds that map for
both.
