---
jurisdiction: au
version: 1
created: 2026-11-19
instruments:
  - name: Spam Act 2003 (Cth)
    effective: 2004-04-10
    provision: "ss. 16-18"
  - name: Spam Regulations 2021 (Cth)
  - name: "Privacy Act 1988 (Cth), Schedule 1"
    effective: 2014-03-12
    provision: "APP 7"
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
    triggers: au/fixtures/advertising-without-objection-route.md
    clears: au/fixtures/advertising-complete.md
  - finding: advertiser_contact
    triggers: au/fixtures/advertising-without-advertiser-contact.md
    clears: au/fixtures/advertising-complete.md
---

# What Australian law requires of an outbound message

This layer rests on the Spam Act 2003, ss. 16 to 18, and the Spam Regulations 2021. Australian
Privacy Principle 7 in the Privacy Act 1988 adds the rules on direct marketing.

## Advertising needs consent (s. 16)

A commercial electronic message with an Australian link needs the recipient's consent. The core
refuses advertising without a recorded consent, so this layer adds no key for it.

## Inferred consent (Schedule 2, cl. 2)

The Act also accepts consent that can be reasonably inferred from conduct and from a business
relationship. The engine models this route as the existing-customer exception, and asks for each of its
conditions:

- a recorded sale to this contact;
- a notice offering a free objection when the address was collected;
- advertising for goods similar to what was bought;
- no standing objection.

The Act names neither similarity nor the notice at collection. The layer asks for both, so it
opens fewer sends than inferred consent does.

## Disclosures

- s. 17: a commercial message names the business that authorised it and says how to reach it
  (`advertiser_contact`).
- s. 18: a commercial message carries a working unsubscribe facility (`objection_route`).
- APP 7.3: direct marketing gives a simple way to opt out.

The Act does not ask for a subject label or limit how often one may send.

## What the engine cannot hold

- s. 18 asks that an unsubscribe request is honoured within five working days. The core stops
  advertising when the request is recorded, which meets it.
- The deemed consent for a conspicuously published address (Schedule 2, cl. 4) depends on the
  message's relevance to the recipient's role. The engine cannot check that, so this layer offers no
  route for it.

## Where this layer takes the stricter reading

- Inferred consent carries all four conditions of the existing-customer exception.
- The Spam Regulations 2021 state no effective date here, because the commencement date was not
  confirmed.

## Review

This layer is a draft. It states no `reviewed` or `reviewer`, so no citation is shown to a rep until
a legal review stamps it.
