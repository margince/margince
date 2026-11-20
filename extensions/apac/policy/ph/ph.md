---
jurisdiction: ph
version: 1
created: 2026-11-19
instruments:
  - name: Data Privacy Act of 2012 (Republic Act No. 10173)
    effective: 2012-09-08
    provision: "ss. 12 and 16"
  - name: Implementing Rules and Regulations of the Data Privacy Act of 2012
    effective: 2016-09-09
    provision: "s. 34"
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
    triggers: ph/fixtures/advertising-without-objection-route.md
    clears: ph/fixtures/advertising-complete.md
  - finding: controller_identity
    triggers: ph/fixtures/notice-without-postal-address.md
    clears: ph/fixtures/notice-complete.md
  - finding: privacy_contact
    triggers: ph/fixtures/notice-without-privacy-contact.md
    clears: ph/fixtures/notice-complete.md
---

# What Philippine law requires of an outbound message

The Philippines has no statute on advertising email. This layer rests on the Data Privacy Act of
2012 (Republic Act No. 10173) and its Implementing Rules and Regulations (IRR).

## Advertising needs consent

s. 12 of the Act lists the grounds for processing. The core refuses advertising without a recorded
consent, and the layer states `marketing-exceptions` empty.

## Disclosures

- Act s. 16(b) and IRR s. 34(a): before processing, the data subject is told who the controller is
  and how to reach it. A Philippine controller carries `controller_identity` and `privacy_contact`
  on every first message.
- IRR s. 34(b): the data subject can object to processing for direct marketing. Advertising carries
  an `objection_route`.

## What this layer does not state

- Act s. 12(f) includes legitimate interests as a ground, so `no-legitimate-interests` is off.
- Act s. 6 reaches some processing abroad of the data of a Philippine citizen or resident. The pack
  does not set `data-protection-follows-recipient`, because that key would also move request
  deadlines and the privacy page to this pack, which states neither.

## Where this layer takes the stricter reading

- Direct marketing on legitimate interests is not settled under the Act. The layer asks for
  consent.

## Review

This layer is a draft. It states no `reviewed` or `reviewer`, so no citation is shown to a rep until
a legal review stamps it.
