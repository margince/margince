<!-- prose:plain -->
# The Vietnamese pack

What Vietnamese law requires of the outbound messages of this product, stated as data that the core
engines apply. The pack decides nothing: it declares, and the engine reads.

All of it comes from **Decree 91/2020/ND-CP** on spam messages, email and calls.

## Advertising email needs prior consent, with no exception

Art. 10 allows an advertising email only where the recipient has given prior consent. The decree
grants **no** route that comes from a sale: nothing here matches the German exception for a current
customer (UWG §7(3)).

So this pack declares an empty exception list. `TestAdvertisingNeedsPriorConsent` holds it, because
the result reaches across countries. The core engine folds the rule sets that apply, and the
strictest one wins. So an exception declared here would let evidence of a German sale approve a
Vietnamese advertising message.

The consent itself must be provable in three ways. These are what was agreed to, how it was given,
and how often the recipient agreed to hear from us. The core consent engine records all three on the proof
row; this pack does not state them again.

## The `[QC]` subject label (Art. 12)

An advertising email is labelled as advertising in its subject line, with the label the decree
fixes: `[QC]`.

The engine checks the finished subject before the provider is called. An advertising message whose
subject does not begin with `[QC]` (in any case) is parked and not sent. The check applies to
advertising only: a message that is not advertising but has an advertising label would describe
itself wrong to the recipient.

## Who is advertising (Art. 13)

An advertising message names the advertiser and gives a way to reach them: name, phone, email,
address and website.

This sits next to the controller disclosures shaped by GDPR, and does not replace them. The
controller and the advertiser are often the same organisation, but need not be, and the two duties
come from different laws.

| Disclosure | Scope |
|---|---|
| Controller identity | every first message |
| Privacy contact | every first message |
| Objection route | advertising |
| Advertiser contact: name, phone, email, address, website | advertising |

## Three per address per day (Art. 22(2))

At most **3** advertising emails reach one address in any rolling **24 hours**, unless that recipient
has agreed to a different frequency.

The count covers messages the recipient **got or is about to get**. A staged message that parked,
and a decision taken in observe mode, both describe a message no one received. To count either would
use up someone's allowance. So the engine counts sent deliveries joined to their advertising
decision, never decision rows on their own.

It also counts messages in flight. An authorization commits before the provider is called, and a
delivery is marked sent afterwards. So between the two, a message is going out that no sent row
reports yet. If only delivered mail counted, every worker in that window would read the same number
and send. The limit would then be passed by however many workers were running.

Both parts are in core, and tests hold them with the mutations "count decisions instead" and "drop
the in-flight arm". The pack only states the limit.

**The limit refuses in every rollout mode.** The other rules of the engine can run in observe while
they are measured against the old consent gate. This one cannot.

An installation that declares a
country states which law it sends under. A setting that let it pass that country's statutory limit
would make the declaration false. Waiting clears it, because the window rolls and the same message
becomes lawful. So to refuse costs a delay, not the message.

## An opt-out is acknowledged (Art. 16)

A recipient who refuses more advertising is owed a confirmation that the refusal was received. It
comes within 24 hours, and holds no advertising of its own.

The acknowledgement goes out through the controller lane. That is the one lane that may write to
someone who has just suppressed themselves, because the message serves the subject and not the
sender. The consent module queues it from the `optout_acknowledgement` controller template, in the
same transaction as the refusal.

No gate holds the 24 hours. Nothing measures the gap or alerts on it. So a worker outage longer than
a day sends the acknowledgement late, with no error.

## Windows

A reply stays a reply for **12 months**; a live deal supports a follow-up no one asked for during
**6 months**. These are the core defaults, stated again so the pack says what it applies. Neither one
limits a reply in the same thread.

## Retention

None. The core retention engine reads the classes of a pack as statutory floors on records the
product holds. The record-keeping duties of Vietnam fall on accounting books and invoices, which a
CRM does not hold. A floor that no stored record falls under would hold nothing, so this pack
declares none.

## Changing any of this

`vn_test.go` checks every claim above. A changed label, limit, disclosure or window is a change to
legal content. Edit the test in the same commit, and say in the PR body which article moved.
