# The record nouns

One rule covers both record nouns: the reader and the program say the same
word, and the schema follows.

| The record | What everything calls it | Retired words |
|---|---|---|
| A human being an installation does business with | `contact` | `person`, `people`, `persons` |
| A company an installation does business with | `company` | `organization`, `org`, `account` |

"Everything" is the whole stack. On the wire that is the URL, the HTTP path, the
operation id, the public event type, the search DSL's traverse relation and the
agent tool's `record_type` argument. In the tree it is the table, the column,
the constraint, the index, the Go identifier, the file name, the i18n key and
the prose in `docs/`.

## Why one word everywhere

One word per record across screen, API and schema means nobody translates
between them. A split costs something on every surface:

- A reader grepping for the contact table finds nothing and concludes there is
  none.
- A model asking for `record_type: "company"` is refused by a registry that
  admits only `"organization"`.
- The browser says `#/contacts/:id` while the request it fires says
  `GET /people/{id}`, so every new surface has to learn they are the same thing.
- A third spelling appears without anybody deciding on it, as when the search
  DSL's traverse relation was derived from the kind word and read `persons`.

Both nouns follow the same rule, and the word is the one on the screen. A
program is easier to read when it agrees with the screen.

## The words that are not these records

These words are not the record nouns. Do not rename them.

**Not a contact.** `personal`: a counterparty verdict, a mail posture, a purge
scope, an exclusion container. `in_person`: a consent qualifying event's kind,
where the word means physically present. `personality_md`: a voice profile's
prose. `personnel`: a confidentiality class. `persona`, `impersonate`,
`salesperson`. In German, `Personen` and `personenbezogene Daten` are the legal
term and are not this record. **Settings → People** is the seats group, beside
Company and Sales. It heads the pages about colleagues with a licence, and it is
the one surface where the plural still means the humans who work here.

**Not a company.** A meeting `organizer`, the adjective `organizational`, a
`.org` TLD, Microsoft's `organizations` authority alias (a literal path segment
at `login.microsoftonline.com`; renaming it stops sign-in working), vCard's
`ORG` property, and schema.org's own type names.

**Spelled `contact` but not this record.** `graph_contact_edge` links two
contacts observed on one thread, and with `contact_a` and `contact_b` it reads
as what it is. Also `organization_fact.field = 'contact_email'`,
`partner.last_contact_at`, `intro_request.route_type = 'through_contact'`, and
a lead whose status is `contacted`.

## What holds it

Three censuses, each deriving its corpus from the tree:

- `frontend/src/i18n/record-noun.test.ts` checks the copy in both directions.
  Every key that offers the record type carries the contact noun. No catalog
  value says the retired noun unless its key is listed there as one where the
  word means a human being, grouped by the reason it stays.
- `backend/gates/contactvocabulary_test.go` checks the code. It fails on the
  retired word in the path or contents of every git-tracked file. It derives
  the human-sense sentences from the catalogs above instead of keeping a second
  copy. The sibling noun has a census of the same shape.
- `backend/internal/shared/gatekit/migrationrenames.go` is not a gate itself.
  A census that reads migration text sees the name a thing was created under,
  so every census reads through the `ALTER ... RENAME` statements the
  migrations declare. Without it the others would miss renamed objects.

Audit rows keep the retired word forever. `trg_audit_no_mutate` refuses an
UPDATE on `audit_log`, which is what makes the trail trustworthy, so a mutation
recorded before a rename still says `entity_type = 'person'` or
`'organization'`. The two read paths that filter the trail by record type accept
both words, and `backend/internal/compose/auditlegacytype.go` holds that mapping
for both.
