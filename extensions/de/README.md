# The German pack

What German law requires of this product, stated as data the core engines apply.
The pack decides nothing: it declares, and the engine reads.

Two obligations live here.

## Retention floors (GoBD, §147 AO)

| Class | Keep | Anchored at |
|---|---|---|
| `commercial_correspondence` | 6 years | end of the calendar year |
| `accounting_records` | 8 years | end of the calendar year |

The core retention engine treats these as **floors**: a workspace policy may
keep longer, never destroy earlier. Anchoring at calendar-year end is §147(4)
AO, and it matters — a January Handelsbrief keeps almost seven calendar years,
so a floor that counted from the record's own date would erase it early.

Bücher and Abschlüsse (10 years) are deliberately absent. A CRM holds no books
or annual accounts, and a floor no record can carry would be documentation
posing as enforcement.

## Outbound messaging (UWG §7, GDPR Art. 13)

### Advertising without consent — the §7(3) existing-customer exception

This is the only route, and it carries **all four** of its statutory conditions:

| Condition | What §7(3) requires |
|---|---|
| Sale evidence | the address was obtained *in connection with a sale* |
| Collection-time opt-out | the customer was told at collection they may object at any time, free beyond transmission cost |
| Similarity | the advertising is for the seller's *own similar* goods |
| No objection | no objection stands |

Declaring three of four would be an exception the engine applies while checking
less than the statute asks. That is worse than declaring none, because it looks
lawful.

**Similarity is checked per message, not once per person.** A customer who
bought one product has not opened the door to everything the seller sells, and
an exception evaluated once per person is the shape that turns one purchase into
a permanent mailing list.

### What a first message discloses (Art. 13)

| Disclosure | Scope |
|---|---|
| Controller identity — legal name and postal address | every first message |
| Privacy contact | every first message |
| Objection route — free and without a barrier | advertising |

The objection route is marketing-scoped because §7(3) requires it at *every*
use, not only the first contact.

### Windows

A reply stays a reply for **12 months**; a live deal supports an unprompted
follow-up for **6 months**.

Neither bounds a same-thread reply. The subject wrote to us and did not
withdraw, so a rep answering a months-old thread is doing the ordinary thing —
these windows reach only an *unprompted* follow-up. A pack that shortened them
would be refusing correspondence rather than restricting advertising.

### What Germany does not require

No subject-line prefix on commercial email, no statutory frequency ceiling, and
no acknowledgement owed for an opt-out.

The zero values say so, and the silence is deliberate: a reader comparing this
pack against one that *does* impose them needs to tell a considered absence from
a forgotten field. `TestGermanyImposesNoPrefixNoCapAndNoAcknowledgement` is what
makes that difference hold.

## Changing any of this

Every claim above is asserted in `de_test.go`. A changed span, condition,
disclosure or window is a legal-content change: edit the test in the same commit
and say in the PR body which statute moved.
