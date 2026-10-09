<!-- prose:plain -->
# The German pack

What German law requires of this product, stated as data that the core engines apply. The pack
decides nothing: it declares, and the engine reads.

Two obligations are here.

## Retention floors (GoBD, §147 AO)

| Class | Keep | Anchored at |
|---|---|---|
| `commercial_correspondence` | 6 years | end of the calendar year |
| `accounting_records` | 8 years | end of the calendar year |

The core retention engine treats these as **floors**: a workspace policy may keep data longer, but
never remove it earlier. To count from the end of the calendar year is §147(4) AO. A Handelsbrief
from January is kept almost seven calendar years. So a floor that counted from the record's own date
would erase it too early.

Bücher and Abschlüsse (10 years) are absent. A CRM holds no books or annual accounts, and a floor
that no stored record falls under would hold nothing.

## Outbound messages (UWG §7, GDPR Art. 13)

### Advertising without consent: the §7(3) exception for a current customer

**Margince does not offer this exception.** The pack declares it with all four of its statutory
conditions, and the engine refuses it. Nothing on a message names the goods it advertises, so the
similarity condition cannot be checked.

| Condition | What §7(3) requires |
|---|---|
| Sale evidence | the address was obtained *in connection with a sale* |
| Collection-time opt-out | the customer was told at collection they may object at any time, free beyond transmission cost |
| Similarity | the advertising is for the seller's *own similar* goods |
| No objection | no objection stands |

To declare three of four would be an exception that the engine applies while it checks less than
the statute asks. That is worse than to declare none, because it looks lawful.

Similarity would have to be checked per message. A customer who bought one product has not opened
the door to all the seller sells. An exception checked once per contact turns one purchase into a
mailing list that lasts for ever.

### What a first message discloses (Art. 13)

| Disclosure | Scope |
|---|---|
| Controller identity: legal name and postal address | every first message |
| Privacy contact | every first message |
| Objection route: free and without a barrier | advertising |

The objection route is limited to advertising, because §7(3) requires it at every use of the
address for advertising, including the first.

### Windows

A reply stays a reply for **12 months**; a live deal supports a follow-up no one asked for during
**6 months**.

Neither one limits a reply in the same thread. The subject wrote to us and did not withdraw, so a
rep who answers a thread that is months old does the normal thing. These windows reach only a
follow-up no one asked for. A pack that made them shorter would refuse correspondence instead of
limiting advertising.

### What Germany does not require

No prefix on the subject line of a commercial email, no statutory limit on how often you send, and
no acknowledgement owed for an opt-out.

The zero values are intended, so a reader who compares packs can tell a chosen absence from a
missing field. `TestGermanyImposesNoPrefixNoCapAndNoAcknowledgement` holds them.

## Changing any of this

`de_test.go` checks every claim above. A changed span, condition, disclosure or window is a change
to legal content. Edit the test in the same commit, and say in the PR body which statute moved.
