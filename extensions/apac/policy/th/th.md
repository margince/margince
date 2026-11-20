---
jurisdiction: th
version: 1
created: 2026-11-19
instruments:
  - name: Personal Data Protection Act B.E. 2562 (2019)
    effective: 2022-06-01
    provision: "ss. 19, 23, 25 and 32"
  - name: "Computer-Related Crime Act B.E. 2550 (2007), as amended by Act (No. 2) B.E. 2560 (2017)"
    effective: 2017-05-24
    provision: "s. 11"
reply-window: 8760h
deal-follow-up-window: 4368h
marketing-exceptions: []
disclosures:
  - kind: controller_identity
  - kind: privacy_contact
  - kind: objection_route
    marketing-only: true
fixtures:
  - finding: objection_route
    triggers: th/fixtures/advertising-without-objection-route.md
    clears: th/fixtures/advertising-complete.md
  - finding: controller_identity
    triggers: th/fixtures/notice-without-postal-address.md
    clears: th/fixtures/notice-complete.md
  - finding: privacy_contact
    triggers: th/fixtures/notice-without-privacy-contact.md
    clears: th/fixtures/notice-complete.md
---

# What Thai law requires of an outbound message

This layer rests on the Personal Data Protection Act B.E. 2562 (PDPA) and s. 11 of the
Computer-Related Crime Act, as amended in 2017.

## Advertising needs consent

Computer-Related Crime Act s. 11 punishes sending data or email that annoys the recipient without
an easy way to refuse it. PDPA s. 19 asks for consent where no other ground applies. The core
refuses advertising without a recorded consent, and the layer states `marketing-exceptions` empty.

## Disclosures

- PDPA s. 23 and s. 25: the controller tells the data subject who it is and how to reach it. Where
  the data came from another source, it does so at the latest with the first communication. A Thai
  controller carries `controller_identity` and `privacy_contact` on every first message.
- PDPA s. 32 and Computer-Related Crime Act s. 11: the recipient can object to direct marketing and
  refuse more mail easily (`objection_route`).

## What this layer does not state

- PDPA s. 24 includes legitimate interests as a ground, so `no-legitimate-interests` is off.
- PDPA s. 5 reaches a controller abroad that offers goods or services in Thailand. The pack does
  not set `data-protection-follows-recipient`, because that key would also move request deadlines
  and the privacy page to this pack, which states neither.

## Where this layer takes the stricter reading

- Direct marketing on legitimate interests is disputed under the PDPA. The layer asks for consent.

## Review

This layer is a draft. It states no `reviewed` or `reviewer`, so no citation is shown to a rep until
a legal review stamps it.
