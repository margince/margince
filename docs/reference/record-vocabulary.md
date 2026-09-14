# The record nouns

One rule, for both of them: **the reader and the program say the same word, and
the schema follows.**

| The record | What everything calls it | What it used to be called |
|---|---|---|
| A human being an installation does business with | `contact` | `person`, `people`, `persons` |
| A company an installation does business with | `company` | `organization`, `org`, `account` |

"Everything" is the whole stack and not just the screen: the URL, the HTTP path,
the operation id, the public event type, the search DSL's traverse relation, the
agent tool's `record_type` argument, the table, the column, the constraint, the
index, the Go identifier, the file name, the i18n key and the prose in `docs/`.

## Why the rule is this way round

It was settled the other way first. The recorded position was "a person reads
*company*, a program is compiled against *organization*" — one word for the
screen and another for the schema, translated at the boundary. Applied to the
sibling noun it says the screen may say *contact* while the schema keeps
`person`, which is the state this product was in and which cost something every
time:

- A reader grepping for the contact table found nothing and concluded there
  wasn't one.
- A model asking for `record_type: "company"` was refused by a registry that
  admitted only `"organization"`.
- The browser said `#/contacts/:id` and the request it fired said
  `GET /people/{id}`, so every new surface had to learn that they were the same
  thing.
- A third spelling appeared without anybody deciding on it: the search DSL's
  traverse relation was `persons`, because it is derived from the kind word and
  nothing had ever been asked to agree with it.

Two record nouns cannot hold opposite rules — that is what makes every new
surface ask the question again, which is the cost the original rule was written
to remove. So both nouns take the same one, and it is the human word: the
product's vocabulary is the vocabulary a reader already has, and a program is
easier to read when it agrees with the screen than when it is one translation
away from it.

## The words that are not these records

A blind substitution breaks each of these, and each survives on purpose.

**Not a contact.** `personal` — a counterparty verdict, a mail posture, a purge
scope, an exclusion container. `in_person` — a consent qualifying event's kind,
where the word means physically present. `personality_md` — a voice profile's
prose. `personnel` — a confidentiality class. `persona`, `impersonate`,
`salesperson`. In German, `Personen` and `personenbezogene Daten` are the legal
term and are not this record. **Settings → People** is the SEATS group, beside
Company and Sales: it heads the pages about colleagues with a licence, and it is
the one surface where the plural still means the humans who work here.

**Not a company.** A meeting `organizer`, the adjective `organizational`, a
`.org` TLD, Microsoft's `organizations` authority alias (a literal path segment
at `login.microsoftonline.com`, and renaming it stops sign-in working), vCard's
`ORG` property, and schema.org's own type names.

**Already spelled `contact` before the rename, and not this record.**
`graph_contact_edge` (two contacts observed on one thread — it keeps its name,
because with `contact_a` and `contact_b` it now reads as what it is),
`organization_fact.field = 'contact_email'`, `partner.last_contact_at`,
`intro_request.route_type = 'through_contact'`, and a lead whose status is
`contacted`.

## What holds it

Three censuses, each deriving its corpus from the tree rather than from a list:

- `frontend/src/i18n/record-noun.test.ts` — the copy, in both directions. Every
  key that offers the record TYPE carries the contact noun, and no catalog value
  says the retired noun unless its key is listed there as one where the word
  means a human being, grouped by the reason it stays.
- `backend/gates/contactvocabulary_test.go` — the code. Every git-tracked file,
  path and contents, fails on the retired word. It derives the human-sense
  sentences from the catalogs above rather than keeping a second copy of them.
  The sibling noun takes a census of the same shape.
- `backend/internal/shared/gatekit/migrationrenames.go` — not a gate itself, but what
  stops the others going quiet: a census that reads migration TEXT reads the
  name a thing was CREATED under, so every one of them reads through the
  `ALTER ... RENAME` statements the migrations themselves declare.

Audit rows keep the retired word forever. `trg_audit_no_mutate` refuses an
UPDATE on `audit_log`, which is the property that makes the trail worth having,
so a mutation recorded before a rename still says `entity_type = 'person'` or
`'organization'`. The two read paths that filter the trail by record type accept
both words, and `backend/internal/compose/auditlegacytype.go` says so once
instead of twice.
