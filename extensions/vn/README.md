<!-- prose:plain -->
# The Vietnamese pack

What Vietnamese law requires of the outbound messages of this product, stated as data that the core
engines apply. The pack decides nothing: it declares, and the engine reads.

Most of it comes from **Decree 91/2020/ND-CP** on spam messages, email and calls. The rules on
advertising content come from the `Law on Advertising`.

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

## The `[QC]` subject label (Art. 18)

An advertising email is labelled as advertising in its subject line, with the label the decree
fixes: `[QC]`.

The engine checks the finished subject before the provider is called. An advertising message whose
subject does not begin with `[QC]` (in any case) is parked and not sent. The check applies to
advertising only: a message that is not advertising but has an advertising label would describe
itself wrong to the recipient.

## Who is advertising (Art. 19)

An advertising message names the advertiser and gives a way to reach them: name, phone, email,
address and website.

This sits next to the controller disclosures shaped by GDPR, and does not replace them. The
controller and the advertiser are often the same organisation, but need not be, and the two duties
come from different laws.

| Disclosure | Scope |
|---|---|
| Controller identity | every first message |
| Privacy contact | every first message |
| Objection route: web page, email and phone | advertising |
| Advertiser contact: name, phone, email, address, website | advertising |

## How to refuse (Art. 20)

A recipient must have three ways to refuse advertising: a web page or a network page, an email
address, and a phone number. So the objection route of the pack names all three. A reply word such as STOP
may stay as an extra.

Where this pack is in use, settings refuse an objection route that leaves out one of the three, and
the error names the missing part. The pack is in use when the installation, or an entity it sends
for, is in Vietnam. At send, a route without all three counts as missing, so the advertising mail
is refused.

A route that meets the rule:

```text
trả lời email này với từ STOP, gửi email đến privacy@example.com, gọi số +84 00 0000 0000 hoặc dùng trang [đường dẫn trang tùy chọn nhận tin].
```

## Company details in Vietnamese

A reader must understand what they act on (`Law 91/2025/QH15` Art. 9.2). So the controller details
have a Vietnamese version of each text field. A Vietnamese mail or page shows the Vietnamese text where
it is stated, and the default text where it is not.

The language of the pack is Vietnamese. Before an advertising mail goes to a Vietnamese
recipient, the objection route and the advertiser contact must be stated in Vietnamese. Until then the
send is refused. The name and the address may stay in the default text. An installation whose own
language is Vietnamese meets this with its default text.

## The end of an advertising mail (Art. 19 and 20.1)

An advertising mail ends with the refusal section. It says that the recipient may refuse all advertising
from the advertiser, then gives the objection route and the opt-out links. Directly before it stands the
block of the advertiser: name, address, phone, email and website.

The same section tells the recipient that they choose if their data is used for advertising, on the
page the settings link opens. This meets `Law on Protection of Consumers' Rights` Art. 10.1(b) and
18.4(b).

## Vietnamese first, and no claim to be the best without proof

`Law on Advertising` Art. 18 asks for advertising content in Vietnamese first. Text in another language
comes below it, and its size is at most three fourths of the Vietnamese text. Art. 8.11 of that law and
`Circular 12/2026/TT-BVHTTDL` Art. 3 limit a claim to a leading, unique or absolute position, in any
language. In Vietnamese that is "nhất", "duy nhất", "tốt nhất", "số một" or "hàng đầu". In English
it is "best", "No. 1", "number one", "leading", "the only" or "unique". A mail may make such a claim
only with a listed proof.
The mail must show the name, number and date of the proof document.

Both are checked while a mail is written, and neither stops the send:

- A draft whose first paragraph in a known language is not Vietnamese gets a note.
- A draft that uses a word of `superlatives()` in `vn.go` gets a note, unless a line holds a document
  number and a date.

The check leaves out a closing greeting that holds "best", such as "Best regards" (`notClaims()` in
`vn.go`). It looks for "the only" and not "only", because a bare "only" most often sets a limit, as in
"only three days left".

The size of text in another language is not checked. A plain text mail has one size, and the check
does not read the styles of an HTML part.

## Advertisers from other countries (Law on Advertising Art. 39.2)

A foreign organisation that does not operate in Vietnam must advertise there through a Vietnamese
advertising service business. The advertiser is the customer of the installation, and the customer is
responsible for this.

So the pack sets `ForeignAdvertiserNeedsLocalAgency`. When the legal entity a mail goes out for is not
established in Vietnam, a draft of advertising mail to a Vietnamese recipient gets a note that says so.
The note does not stop the send.

## The sending server

No rule in force requires advertising email to be sent from a server in Vietnam, so the pack blocks
nothing on it. `Decree 330/2026/ND-CP` Art. 38.3(a) fines product information sent by email from a
server outside Vietnam. The rule it enforces was Art. 17.2(b) of `Decree 90/2008/ND-CP`, as amended
by `Decree 77/2012/ND-CP`. `Decree 91/2020/ND-CP` replaced that decree from 1 October 2020 (Art. 35.2),
and it has no such rule.

The installation can send through its own SMTP relay (`email.smtp.*`). For advertising mail to
Vietnamese recipients, use a relay located in Vietnam where one is available.

## When marketing mail to Vietnamese recipients is cleared

Marketing mail to Vietnamese recipients is cleared on these conditions:

1. The `Decree 91/2020/ND-CP` rules in this pack stay on: prior consent, the `[QC]` label, the
   advertiser block directly before the refusal section, and at most 3 mails in 24 hours.
2. Vietnamese-first content and the superlative check (`Law on Advertising` Articles 18 and 8.11;
   `Circular 12/2026/TT-BVHTTDL` Article 3).
3. The sending server: no rule in force requires a server in Vietnam. Use an SMTP relay located in
   Vietnam where available.
4. A customer that is a foreign organisation not operating in Vietnam must advertise in Vietnam
   through a Vietnamese advertising service business (`Law on Advertising` Article 39.2). The pack
   warns, and the customer is responsible.

Consent to advertising is asked for its own purpose and never together with another consent. Each
page a contact grants consent on grants the one purpose it asks about.

This is the legal position as of 16 November 2026.

## Three per address per day (Art. 13.5)

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

## An opt-out is acknowledged (Art. 20)

A recipient who refuses advertising email is owed a receipt at once (Art. 20.3). It states when the
refusal reached the sender and when sending stopped (Art. 20.4(a)), and holds no advertising
(Art. 20.4(b)).

The consent module queues the receipt from the `optout_acknowledgement` controller template, in the
same transaction as the refusal. The send job runs once that transaction commits. It goes
out through the controller lane. That is the one lane that may write to someone who has just
suppressed themselves, because the message serves the subject and not the sender.

The receipt prints two times, both in Vietnam time (UTC+07:00), which this pack declares:

- **Received**: when the refusal reached the sender. For a link press it is the press. For a reply
  mail or a call that a colleague records, it is the `received_at` they enter, never the time they
  record it.
- **Stopped**: when the stop was recorded, which is when sending stopped.

The receipt carries no "Powered by Margince" line, since Art. 3.1 counts a line that names a third
party's product as advertising. The other controller mails keep it.

One refusal has one receipt: its message id comes from the refusal, and the delivery ledger refuses a
second row with the same id.

The pack sets the receipt's alert time to one hour. A receipt not sent by then raises a
notice to the privacy operators, and **Settings → System health** counts it under **Opt-out
receipts** for 30 days.

## Records kept (Art. 11)

Consents given, refusals and receipts stay at least one year (Art. 11.7). The pack declares the year as
`OptOutRecordsKeep`.

Consent stamps its end on each stop and receipt (`kept_until`). Until that
date, no retention policy changes, deletes or archives the contact, lead or receipt. After it, the
usual policies apply. A request to delete the subject still removes their details.
**Download opt-out records** on a contact's consent panel exports all three when the state asks to
see them (Art. 11.3).

## A consent request answers the contact's own request (Art. 11.2)

A mail that asks a contact to confirm a subscription they never asked for is itself a message they
did not ask for. So this pack declares `ConsentConfirmationNeedsRequest`. A user can send the
consent-confirmation mail only when the contact's own request is on record. That is a request a
user logged, a form the contact sent, or a mail the contact wrote. The core keeps that request on the
link and on the grant's proof row (`consent_event.subject_request_id`). The rule is read in
`backend/internal/modules/consent/confirmrules.go`.

The decree governs advertising, so the rule covers a request for consent to a purpose of the
marketing class only (`ConsentRequestFirstFor`). A request for consent to correspondence advertises
nothing, so a user may send it unasked. That is the request the
**Ask for consent** button of the no-basis warning sends, and its proof row names no request of the
contact's own.

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
