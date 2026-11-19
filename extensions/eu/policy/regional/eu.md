---
created: 2026-11-19
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
address from a person's address, so every pack here asks for consent for both.

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

Every pack in this folder is a draft: no layer states `reviewed` or `reviewer`, so no citation is
shown to a rep. A legal review stamps them.
