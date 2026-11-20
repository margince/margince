---
jurisdiction: kr
version: 1
created: 2026-11-19
instruments:
  - name: "Act on Promotion of Information and Communications Network Utilization and Information Protection"
    effective: 2014-11-29
    provision: "art. 50"
  - name: "Enforcement Decree of the Act on Promotion of Information and Communications Network Utilization and Information Protection"
    provision: "arts. 61 and 62-2"
reply-window: 8760h
deal-follow-up-window: 4368h
marketing-exceptions: []
consent-only: true
disclosures:
  - kind: objection_route
    marketing-only: true
  - kind: advertiser_contact
    marketing-only: true
subject-prefix: "(광고)"
opt-out-acknowledgement: true
opt-out-acknowledgement-within: 336h
opt-out-acknowledgement-utc-offset: 9h
consent-confirmation-needs-request: true
consent-request-first-for: [marketing, phone_outreach]
fixtures:
  - finding: objection_route
    triggers: kr/fixtures/advertising-without-objection-route.md
    clears: kr/fixtures/advertising-complete.md
  - finding: advertiser_contact
    triggers: kr/fixtures/advertising-without-advertiser-contact.md
    clears: kr/fixtures/advertising-complete.md
  - finding: subject_prefix
    triggers: kr/fixtures/advertising-without-label.md
    clears: kr/fixtures/advertising-complete.md
---

# What Korean law requires of an outbound message

This layer rests on art. 50 of the Act on Promotion of Information and Communications Network
Utilization and Information Protection (정보통신망법), and on arts. 61 and 62-2 of its Enforcement
Decree. Since 29 November 2014, advertising by email needs prior consent.

## Advertising needs prior consent (art. 50(1))

Advertising information sent for profit needs the recipient's express prior consent.
`consent-only` adds that no one-message exception a sender or an admin records takes its place.

Art. 50(1)(i) lets a seller who collected the address in a transaction advertise the same kind of
goods for six months after it. The engine cannot check the six months or the kind of goods, so this
layer offers no exception and states `marketing-exceptions` empty.

## The subject label

The Enforcement Decree asks that an advertising subject starts with `(광고)`. A subject without it is
held.

## Disclosures (art. 50(4))

Advertising names the sender and how to reach them (`advertiser_contact`). It also gives a free way
to refuse more advertising or withdraw consent (`objection_route`, art. 50(4) and (6)).

## The result of a refusal (art. 50(7))

A recipient who refuses, consents or withdraws consent is told the result within 14 days
(Enforcement Decree art. 62-2). The pack owes an opt-out receipt. An unsent receipt raises its
alarm after 336 hours, which is 14 days, and prints its times in Korean time (UTC+9).

The engine sends the receipt on a refusal or a withdrawal. It sends none when a consent is given,
because the rules contract has no field for that receipt.

## Consent requests

This layer reads a mail that asks for consent to advertising as advertising information itself. So a
user sends a consent request for the advertising classes only when the recipient's own request is on
record.

## What the engine cannot hold

- Art. 50(3) asks for separate consent to advertise between 21:00 and 08:00. Enforcement Decree
  art. 61 exempts email from that rule, and the rules contract has no field for sending hours.
- Art. 50(8) asks the sender to confirm a standing consent every two years. The rules contract has
  no field for it.

## Where this layer takes the stricter reading

- The transaction exception of art. 50(1)(i) is left out, because its six months and its kind of
  goods cannot be checked.
- A consent request needs the recipient's own request first. The Act states no such rule, and
  whether a consent request by email is itself advertising was not confirmed.
- The email exemption from the night-time rule is the reading of Enforcement Decree art. 61. It was
  not confirmed against the current text.

## Review

This layer is a draft. It states no `reviewed` or `reviewer`, so no citation is shown to a rep until
a legal review stamps it.
