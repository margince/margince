---
jurisdiction: nz
version: 1
created: 2026-11-19
instruments:
  - name: Unsolicited Electronic Messages Act 2007
    effective: 2007-09-05
    provision: "ss. 9-11"
    source-url: https://www.legislation.govt.nz/act/public/2007/0007/latest/whole.html
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
    triggers: nz/fixtures/advertising-without-objection-route.md
    clears: nz/fixtures/advertising-complete.md
  - finding: advertiser_contact
    triggers: nz/fixtures/advertising-without-advertiser-contact.md
    clears: nz/fixtures/advertising-complete.md
citations:
  - code: no_marketing_consent
    territorial-test: any_sender
    instrument: "Unsolicited Electronic Messages Act 2007"
    provision: "s. 9"
  - code: marketing_permission_required
    territorial-test: any_sender
    instrument: "Unsolicited Electronic Messages Act 2007"
    provision: "s. 9"
  - code: marketing_consent_required
    territorial-test: any_sender
    instrument: "Unsolicited Electronic Messages Act 2007"
    provision: "s. 9"
  - code: unconfirmed_double_opt_in
    territorial-test: any_sender
    instrument: "Unsolicited Electronic Messages Act 2007"
    provision: "s. 9"
  - code: advertiser_contact
    territorial-test: any_sender
    instrument: "Unsolicited Electronic Messages Act 2007"
    provision: "s. 10"
  - code: objection_route
    territorial-test: any_sender
    instrument: "Unsolicited Electronic Messages Act 2007"
    provision: "s. 11"
  - code: public_opt_out_link
    territorial-test: any_sender
    instrument: "Unsolicited Electronic Messages Act 2007"
    provision: "s. 11"
---

# What New Zealand law requires of an outbound message

This layer rests on the Unsolicited Electronic Messages Act 2007, ss. 9 to 11.

## Advertising needs consent (s. 9)

A commercial electronic message with a New Zealand link needs the recipient's consent. The core
refuses advertising without a recorded consent, so this layer adds no key for it.

## Inferred consent (s. 4)

The Act accepts consent that can be reasonably inferred from conduct and from business
relationships. The engine models this route as the existing-customer exception, and asks for each of its
conditions:

- a recorded sale to this contact;
- a notice offering a free objection when the address was collected;
- advertising for goods similar to what was bought;
- no standing objection.

The Act names neither similarity nor the notice at collection. The layer asks for both, so it
opens fewer sends than inferred consent does.

## Disclosures

- s. 10: a commercial message names who authorised it and how to reach them
  (`advertiser_contact`).
- s. 11: a commercial message carries a working unsubscribe facility (`objection_route`).

The Act does not ask for a subject label or limit how often one may send.

## What the engine cannot hold

- s. 11 asks that an unsubscribe request is honoured within five working days. The core stops
  advertising when the request is recorded, which meets it.
- Deemed consent for a conspicuously published address (s. 4) depends on the message's relevance to
  the recipient's role. The engine cannot check that, so this layer offers no route for it.

## Where this layer takes the stricter reading

- Inferred consent carries all four conditions of the existing-customer exception.

## Review

This layer is a draft. It states no `reviewed` or `reviewer`, so no citation is shown to a rep until
a legal review stamps it.
