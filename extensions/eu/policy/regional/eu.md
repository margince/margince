---
created: 2026-11-19
source-checked: 2026-11-25
instruments:
  - name: Directive 2002/58/EC
    effective: 2002-07-31
    provision: "Art. 13"
    source-url: https://eur-lex.europa.eu/eli/dir/2002/58/oj
    text-url: https://publications.europa.eu/resource/cellar/6def8269-d337-4373-83c1-8c57678fc93f.0006.03/DOC_3
    text-digest: 82ec515ca024e3d8957be8fef1ff496463afa486cdcce67e7c3f855812e2fb41
    text-fetched: 2026-11-25
  - name: "Regulation (EU) 2016/679 (GDPR)"
    effective: 2018-05-25
    provision: "Art. 6(1)"
    source-url: https://eur-lex.europa.eu/eli/reg/2016/679/oj
    text-url: https://publications.europa.eu/resource/cellar/5f2552c2-cc45-11e6-ad7c-01aa75ed71a1.0022.03/DOC_1
    text-digest: c2a3cba38046acebdb9e791da174295fd0cd2ed11b5473682084f47848bd3993
    text-fetched: 2026-11-25
reply-window: 8760h
deal-follow-up-window: 4368h
marketing-exceptions:
  - kind: existing_customer
    requires-sale-evidence: true
    requires-collection-time-opt-out: true
    requires-similarity: true
    requires-no-objection: true
disclosures:
  - kind: controller_identity
  - kind: privacy_contact
  - kind: objection_route
    marketing-only: true
record-confirmation-without-news: true
fixtures:
  - finding: objection_route
    triggers: fixtures/advertising-without-objection-route.md
    clears: fixtures/advertising-with-objection-route.md
  - finding: objection_route
    triggers: fixtures/advertising-without-objection-route.md
    clears: fixtures/invoice-without-objection-route.md
  - finding: controller_identity
    triggers: fixtures/invoice-without-postal-address.md
    clears: fixtures/invoice-without-objection-route.md
  - finding: privacy_contact
    triggers: fixtures/invoice-without-privacy-contact.md
    clears: fixtures/invoice-without-objection-route.md
citations:
  - code: no_compatible_evidence
    territorial-test: any_sender
    instrument: "Regulation (EU) 2016/679 (GDPR)"
    provision: "Art. 6(1)"
  - code: deal_not_requested
    territorial-test: any_sender
    instrument: "Regulation (EU) 2016/679 (GDPR)"
    provision: "Art. 6(1)"
  - code: marketing_objection
    territorial-test: any_sender
    instrument: "Regulation (EU) 2016/679 (GDPR)"
    provision: "Art. 21(3)"
  - code: objection
    territorial-test: any_sender
    instrument: "Regulation (EU) 2016/679 (GDPR)"
    provision: "Art. 21(3)"
  - code: opted_out_after_request
    territorial-test: any_sender
    instrument: "Regulation (EU) 2016/679 (GDPR)"
    provision: "Art. 21(3)"
  - code: consent_withdrawn
    territorial-test: any_sender
    instrument: "Regulation (EU) 2016/679 (GDPR)"
    provision: "Art. 7(3)"
  - code: withdrawn
    territorial-test: any_sender
    instrument: "Regulation (EU) 2016/679 (GDPR)"
    provision: "Art. 7(3)"
  - code: processing_restricted
    territorial-test: any_sender
    instrument: "Regulation (EU) 2016/679 (GDPR)"
    provision: "Art. 18(2)"
  - code: objection_route
    territorial-test: any_sender
    instrument: "Regulation (EU) 2016/679 (GDPR)"
    provision: "Art. 21(4)"
  - code: controller_identity
    territorial-test: any_sender
    instrument: "Regulation (EU) 2016/679 (GDPR)"
    provision: "Art. 13(1)(a) and Art. 14(1)(a)"
  - code: privacy_contact
    territorial-test: any_sender
    instrument: "Regulation (EU) 2016/679 (GDPR)"
    provision: "Art. 13(1)(b) and Art. 14(1)(b)"
---

# What every EU member state requires of an outbound message

This layer states what the 27 member states share. Each country folder lists it first and then its
own layer, which may only tighten it. A rule that one member state's law does not carry stays out
of this layer.

The layer rests on Directive 2002/58/EC (the ePrivacy Directive), Art. 13, as amended by Directive
2009/136/EC. A directive binds no sender by itself. Each member state carries it into its own law,
so each country layer names the national act that binds there.

## Advertising needs consent (Art. 13(1))

Advertising email to a natural person needs their prior consent. The core refuses advertising
without a recorded consent, so this layer adds no key for it.

Art. 13(5) leaves the rule for legal persons to each member state. Some states ask for consent,
and others ask only that the recipient has not objected. The engine cannot tell a legal person's
address from an individual's address, so every pack here asks for consent for both.

## The existing-customer exception (Art. 13(2))

This exception is the only route to advertising without a recorded consent. It carries all four
of its conditions:

- The address was obtained from the customer in connection with a sale of a product or service.
- The advertising is for the seller's own similar products or services.
- The customer was given a clear chance to object, free of charge and easily, when the address
  was collected.
- No objection stands, and every message offers the chance to object again.

A country layer whose law adds a condition the engine cannot check states no exception at all.
An exception that checks less than the law asks would let the engine send where the law refuses.

## Disclosures (Art. 13(4), GDPR Art. 13 and 14)

A first message says who is writing and who answers about the data. Art. 13(4) forbids an
advertising message that hides the sender, or that gives no valid address to ask for it to stop.
So the objection route binds every advertising message, not only the first.

## The citations

A warning names the law it rests on. This layer cites what the GDPR settles the same way in every
member state:

- the lawful basis a message needs (Art. 6(1));
- the withdrawal of a consent (Art. 7(3));
- a restriction of processing (Art. 18(2));
- an objection to direct marketing (Art. 21(3));
- the information a first message owes (Art. 13, 14 and 21(4)).

Each country layer cites its own national act for the marketing consent and the opt-out address.
One code is cited once, so a country layer cites none of the codes above.

## The windows

The two windows are the core defaults: a reply stays a reply for 8760 hours (365 days), and a live
deal supports a follow-up for 4368 hours (182 days). Neither bounds a reply in the same thread.

## The details check

No member state has settled that a mail which asks a contact to check their details, and also
asks whether they want news, is not itself advertising. This layer takes the stricter reading, so
`record-confirmation-without-news` is true and the check asks no news question.

## Germany

Germany's layer sits on this one like every other member state's. It states the same exception,
disclosures and windows in the words of UWG §7, and they compile to the same rules.

## Review

Every pack in this folder is a draft: no layer states `reviewed` or `reviewer`. Its citations are
shown all the same. The release refresh checks the sources and stamps the packs.
