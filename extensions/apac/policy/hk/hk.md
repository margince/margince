---
jurisdiction: hk
version: 1
created: 2026-11-19
instruments:
  - name: Unsolicited Electronic Messages Ordinance (Cap. 593)
    provision: "ss. 8 to 10"
    source-url: https://www.elegislation.gov.hk/hk/cap593
  - name: "Personal Data (Privacy) Ordinance (Cap. 486), Part 6A"
    effective: 2013-04-01
    provision: "ss. 35C to 35G"
    source-url: https://www.elegislation.gov.hk/hk/cap486
reply-window: 8760h
deal-follow-up-window: 4368h
marketing-exceptions: []
disclosures:
  - kind: objection_route
    marketing-only: true
  - kind: advertiser_contact
    marketing-only: true
fixtures:
  - finding: objection_route
    triggers: hk/fixtures/advertising-without-objection-route.md
    clears: hk/fixtures/advertising-complete.md
  - finding: advertiser_contact
    triggers: hk/fixtures/advertising-without-advertiser-contact.md
    clears: hk/fixtures/advertising-complete.md
---

# What Hong Kong law requires of an outbound message

This layer rests on the Unsolicited Electronic Messages Ordinance (Cap. 593, UEMO) and Part 6A of
the Personal Data (Privacy) Ordinance (Cap. 486, PDPO).

## Advertising needs consent

The UEMO lets a commercial electronic message go out until the recipient unsubscribes. Part 6A of
the PDPO still needs the data subject's consent before a data user uses their personal data in
direct marketing (s. 35E). So the core's refusal of advertising without a recorded consent stands.
The PDPO grants no route that a sale opens, and the layer states `marketing-exceptions` empty.

## Disclosures

- UEMO s. 8: a commercial message carries clear and accurate information that identifies who
  authorised it and how to reach them (`advertiser_contact`).
- UEMO s. 9 and PDPO s. 35F: the message carries an unsubscribe facility, and the first use of the
  data in direct marketing tells the recipient they may refuse (`objection_route`).

## What the engine cannot hold

- UEMO s. 10 asks that an unsubscribe request is honoured within 10 working days. The core stops
  advertising when the request is recorded, which meets it.
- The do-not-call registers of the UEMO cover fax, text and phone numbers, not email.

## Where this layer takes the stricter reading

- The UEMO states no effective date here. Its parts came into force on different dates, and the
  date of the sending rules was not confirmed.

## Review

This layer is a draft. It states no `reviewed` or `reviewer`, so no citation is shown to a rep until
a legal review stamps it.
