# The Vietnamese pack

What Vietnamese law requires of this product's outbound messaging, stated as
data the core engines apply. The pack decides nothing: it declares, and the
engine reads.

Everything here comes from **Decree 91/2020/ND-CP** on anti-spam messages,
email and calls.

## Advertising email needs prior consent — and there is no exception

Art. 10 permits an advertising email only where the recipient has given prior
consent. The decree grants **no** sale-derived route: nothing here corresponds
to the German existing-customer exception (UWG §7(3)).

So this pack declares an empty exception list, and the emptiness is the rule.
`TestAdvertisingNeedsPriorConsent` holds it, because the consequence reaches
across jurisdictions: the core engine folds the applicable rule sets
strictest-wins, and an exception declared here would let evidence of a **German**
sale authorize a Vietnamese advertising message.

The consent itself must be demonstrable in three dimensions — what was consented
to, how it was given, and how often the recipient agreed to hear from us. The
core consent engine records all three on the proof row; this pack does not
restate them.

## The `[QC]` subject label (Art. 12)

An advertising email is labelled as advertising in its subject line, with the
label the decree fixes: `[QC]`.

The engine applies it **exactly once** — a subject that already carries it is
not labelled twice — and applies it to advertising **only**. An operational
message wearing an advertising label misdescribes itself to the recipient in the
other direction, which is the same wrong.

## Who is advertising (Art. 13)

An advertising message names the advertiser and gives a way to reach them:
name, phone, email, address and website.

This sits **alongside** the GDPR-shaped controller disclosures rather than
replacing them. Who is processing someone's data and who is advertising to them
are the same organisation here and need not be, and the two obligations come
from different instruments.

| Disclosure | Scope |
|---|---|
| Controller identity | every first message |
| Privacy contact | every first message |
| Objection route | advertising |
| Advertiser contact — name, phone, email, address, website | advertising |

## Three per address per day (Art. 22(2))

At most **3** advertising emails reach one address in any rolling **24 hours**,
unless that recipient has agreed to a different frequency.

**The count is of messages the recipient got or is about to get.** A staged
message that parked and a decision taken in observe mode both describe a message
nobody received, and counting either would silently consume somebody's
allowance — so the engine counts sent deliveries joined to their advertising
decision, never decision rows on their own.

It also counts messages in flight. An authorization commits before the provider
is called, and a delivery is marked sent afterwards, so between the two there is
a message going out that no sent row reports yet. Counting only delivered mail
would let every worker in that window read the same number and each send, and
the ceiling would be exceeded by however many workers were running.

Both halves live in core and are held by tests whose mutations are exactly
"count decisions instead" and "drop the in-flight arm"; the pack only states the
bound.

**The ceiling refuses regardless of rollout mode.** The engine's other rules can
be run in observe while they are measured against the old consent gate; this one
cannot, because an installation that declares a country is asserting which law
it sends under, and a setting that let it exceed that country's statutory limit
would make the declaration false. Waiting clears it — the window rolls and the
same message becomes lawful — so refusing costs a delay rather than the message.

## An opt-out is acknowledged (Art. 16)

A recipient who refuses further advertising is owed a confirmation that the
refusal was received, within 24 hours, carrying no advertising of its own.

The acknowledgement goes out through the controller lane — the one lane that may
write to somebody who has just suppressed themselves, because the message serves
the subject rather than the sender.

**Not yet built.** The flag is declared and nothing reads it: the controller
template catalog it needs has not landed. Same for the `[QC]` label and the
advertiser disclosure above — both are declared here and applied by nothing
today. What this pack enforces right now is the ceiling and the absence of a
marketing exception.

## Windows

A reply stays a reply for **12 months**; a live deal supports an unprompted
follow-up for **6 months**. These are the core defaults, restated so the pack
says what it applies rather than inheriting silently. Neither bounds a
same-thread reply.

## Retention

**None.** The core retention engine reads a pack's classes as statutory floors
on records the product holds, and Vietnam's record-keeping duties fall on
accounting books and invoices, which a CRM does not hold. A floor no record can
carry would be documentation posing as enforcement.

## Changing any of this

Every claim above is asserted in `vn_test.go`. A changed label, ceiling,
disclosure or window is a legal-content change: edit the test in the same commit
and say in the PR body which article moved.
