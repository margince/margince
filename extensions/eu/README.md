<!-- prose:plain -->
# The EU member-state packs

What the law of each EU member state requires of this product, stated as data that the core engines
apply. The packs decide nothing: they declare, and the engine reads.

Each of the 27 member states has a pack here with its outbound-message rules. The pack for Germany
also holds the retention floors, the request deadline, and the documents an employer signs before mail
is read. Every other member state gets the core's own answer for those: no retention floor, and the
GDPR month for every request.

## The messaging rules of each member state

The rules live in `policy/`, as Markdown with front matter. Each member state has a folder named
for its code, such as `policy/fr/`. Its `index.md` lists two layers:

1. `regional/eu.md`: what every member state shares under Directive 2002/58/EC, Art. 13.
2. `<code>/<code>.md`: the law of that member state, and what it adds.

The shared layer holds the existing-customer exception with its four conditions, the disclosures and
the windows.

A country layer may only make the EU layer stricter. The compiler refuses a layer that makes it
less strict. Each layer body names the act of that member state it rests on. It also names the points where it
takes the stricter reading.

No pack here states a review date yet, so a rep sees no citation from any of them. A legal review
adds the date.

## Germany

The sections below hold for Germany alone.

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

The pack declares the exception with all four of its statutory conditions. The engine checks each
one before a send, in `backend/internal/modules/consent/existingcustomerroute.go`. The mail must go
to an address that was on the buyer's record before the sale. An address added later gets no
exception.

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

## The details check asks nothing about news (UWG §7(2) no. 2)

A mail can ask a contact to check their details and also ask for marketing consent. German law
does not yet say whether such a mail is itself advertising. So this pack declares
`RecordConfirmationWithoutNews`. The details check then goes out with the `record_check` wording,
which names no news, and its page asks no news question. The rule is read in
`backend/internal/modules/consent/confirmrules.go`. It goes when German legal advice confirms that the
combined mail is not advertising.

## Request deadlines (GDPR Art. 12(3))

Every kind of GDPR request is due one calendar month after it arrives. German
law sets no shorter deadline, and the pack states the month with its citation.

## Documents

`documents/de/de/` holds the five originals to sign; `documents/de/en/` holds the English copies,
for reading only. The list is in [docs/compliance/README.md](../../docs/compliance/README.md). The
Jurisdiction settings page lists and serves them when the installation is set to Germany.

## Changing any of this

The steps to change a rule or add a country are in
[Update a messaging policy pack](../../docs/how-to/update-a-messaging-policy-pack.md). Which part of
the product reads each rule is in [Where a pack's rules are read](../../docs/explanation/messaging-policy-packs.md).

`de_test.go` checks every claim about Germany above, and `eu_test.go` checks that every member state
has one pack. A changed span, condition, disclosure or window is a change
to legal content. Edit the test in the same commit, and say in the PR body which statute moved.

`gen-messaging-rules` compiles `policy/` into `messagingrules_gen.go`: run `go generate` here, or
`make -C backend gen`. Each policy file names pairs of fixture drafts, in `policy/fixtures/` or in
its country's own `fixtures/`. One draft of a pair must raise a finding, and one must not. The consent
tests in the backend run each pair through the engine's own checks.
