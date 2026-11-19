---
jurisdiction: jp
version: 1
created: 2026-11-19
instruments:
  - name: "Act on Regulation of Transmission of Specified Electronic Mail (Act No. 26 of 2002)"
    effective: 2002-07-01
    provision: "arts. 3 and 4"
    source-url: https://elaws.e-gov.go.jp/document?lawid=414AC0000000026
  - name: "Act No. 54 of 2008, amending Act No. 26 of 2002"
    effective: 2008-12-01
reply-window: 8760h
deal-follow-up-window: 4368h
marketing-exceptions:
  - kind: existing_customer
    requires-sale-evidence: true
    requires-collection-time-opt-out: true
    requires-similarity: true
    requires-no-objection: true
disclosures:
  - kind: objection_route
    marketing-only: true
  - kind: advertiser_contact
    marketing-only: true
fixtures:
  - finding: objection_route
    triggers: jp/fixtures/advertising-without-objection-route.md
    clears: jp/fixtures/advertising-complete.md
  - finding: advertiser_contact
    triggers: jp/fixtures/advertising-without-advertiser-contact.md
    clears: jp/fixtures/advertising-complete.md
---

# What Japanese law requires of an outbound message

This layer rests on the Act on Regulation of Transmission of Specified Electronic Mail (特定電子メール法,
Act No. 26 of 2002). Its 2008 amendment, in force from 1 December 2008, made advertising mail need
prior consent.

## Advertising needs consent (art. 3(1))

Specified electronic mail, sent to advertise, needs the recipient's prior consent. The core refuses
advertising without a recorded consent, so this layer adds no key for it.

## The transaction-relationship exception (art. 3(1)(iii))

Art. 3(1) exempts a recipient who has a business relationship with the sender. The engine models this
route as the existing-customer exception, and asks for each of its conditions:

- a recorded sale to this contact;
- a notice offering a free objection when the address was collected;
- advertising for goods similar to what was bought;
- no standing objection (art. 3(3)).

The Act names neither similarity nor the notice at collection. The layer asks for both, so it
opens fewer sends than the Act does.

Art. 3(1) also exempts an address the recipient handed over in writing, such as on a business card,
and an address published for business. The engine cannot tell either from a record, so this layer
offers no route for them.

## Disclosures (art. 4)

Advertising mail names the sender and gives a way to reach them (`advertiser_contact`). It also
names the address or link where the recipient refuses more mail (`objection_route`).

## What this layer does not state

- The 2008 amendment dropped the old 未承諾広告※ subject label. Japanese law asks for no label now.
- Art. 3(2) asks the sender to keep a record of each consent. The core keeps consent proof rows,
  so this layer adds no key for it.
- The Act against Unjustifiable Premiums and Misleading Representations limits what an
  advertisement may claim. The rules contract has no field for a misleading claim, so it is named
  here and not applied.

## Where this layer takes the stricter reading

- The transaction-relationship exception carries all four conditions of the existing-customer
  exception.

## Review

This layer is a draft. It states no `reviewed` or `reviewer`, so no citation is shown to a rep until
a legal review stamps it.
