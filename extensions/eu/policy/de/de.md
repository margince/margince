---
jurisdiction: de
version: 2
created: 2026-09-04
record-confirmation-without-news: true
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
fixtures:
  - finding: objection_route
    triggers: de/fixtures/advertising-without-objection-route.md
    clears: de/fixtures/advertising-with-objection-route.md
  - finding: objection_route
    triggers: de/fixtures/advertising-without-objection-route.md
    clears: de/fixtures/invoice-without-objection-route.md
  - finding: controller_identity
    triggers: de/fixtures/invoice-without-postal-address.md
    clears: de/fixtures/invoice-without-objection-route.md
  - finding: privacy_contact
    triggers: de/fixtures/invoice-without-privacy-contact.md
    clears: de/fixtures/invoice-without-objection-route.md
---

# What German law requires of an outbound message

This layer states the rules as data. The core engine applies them, and nothing here decides a send.

## The existing-customer exception (UWG §7(3))

This exception is the only route to advertising without a recorded consent. It carries all four
of its statutory conditions:

- The address was obtained in connection with a sale.
- The customer was told at collection that they may object at any time, free of charge beyond the
  cost of transmission.
- The advertising is for the seller's own similar goods.
- No objection stands.

An exception that checks fewer conditions than the statute would let the engine apply less than
the law asks, while it looks lawful. The engine checks similarity per message. One purchase does
not open the door to everything the seller sells.

## The windows

The two windows are the core defaults, stated here so the pack says what it applies.

- `reply-window`, 8760 hours (365 days): how long an inbound message keeps an unprompted follow-up
  a follow-up.
- `deal-follow-up-window`, 4368 hours (182 days): how long a live deal does.

Neither window bounds a reply in the same thread. The type is a duration and a month is not one, so
the windows are written in hours. The small drift against calendar months falls on the permissive
side, because the windows bound an unprompted follow-up and not a refusal.

## Disclosures (GDPR Art. 13)

A first message says who is writing, who answers about the data and how to say stop. The
objection route binds advertising only: §7(3) asks for it on every advertising message, not only
the first.

## What Germany does not require

No subject prefix, no frequency cap and no receipt for an opt-out, so those keys are absent.
A reader who compares this layer with another pack can see which duties Germany does not impose.

## The details check

German law has not settled whether a details check that also asks for news is advertising. So
`record-confirmation-without-news` is true, and the check asks no news question.
