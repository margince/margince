---
jurisdiction: in
version: 1
created: 2026-11-19
instruments:
  - name: Information Technology Act, 2000
    effective: 2000-10-17
  - name: Digital Personal Data Protection Act, 2023
    provision: "ss. 5, 6 and 7"
reply-window: 8760h
deal-follow-up-window: 4368h
marketing-exceptions: []
disclosures:
  - kind: controller_identity
  - kind: privacy_contact
  - kind: objection_route
    marketing-only: true
no-legitimate-interests: true
fixtures:
  - finding: objection_route
    triggers: in/fixtures/advertising-without-objection-route.md
    clears: in/fixtures/advertising-complete.md
  - finding: controller_identity
    triggers: in/fixtures/notice-without-postal-address.md
    clears: in/fixtures/notice-complete.md
  - finding: privacy_contact
    triggers: in/fixtures/notice-without-privacy-contact.md
    clears: in/fixtures/notice-complete.md
---

# What Indian law requires of an outbound message

India has no statute on advertising email. The telemarketing regulations of the telecom regulator
(TRAI) cover calls and text messages, and their Do Not Disturb register does not reach email. The
Information Technology Act, 2000 holds no rule on commercial email. This layer rests on the Digital
Personal Data Protection Act, 2023 (DPDP Act), which governs the personal data a sender uses.

## Advertising needs consent

The core refuses advertising without a recorded consent, so this layer adds no key for it. The
DPDP Act grants no route that a sale opens, so `marketing-exceptions` is stated and empty.

## No legitimate interests (s. 7)

The DPDP Act allows processing on consent or on a short list of legitimate uses in s. 7. A general
legitimate interest is not on it, so `no-legitimate-interests` is set.

## Disclosures

- s. 5: a request for consent comes with a notice that names the data, the purpose, and how to
  exercise rights and complain. An Indian controller carries `controller_identity` and
  `privacy_contact` on every first message.
- s. 6(4): consent can be withdrawn as easily as it was given. Advertising carries an
  `objection_route`.

## What this layer does not state

- s. 3 reaches a processor abroad that offers goods or services in India. The pack does not set
  `data-protection-follows-recipient`, because that key would also move request deadlines and the
  privacy page to this pack, which states neither.

## Where this layer takes the stricter reading

- The DPDP Act states no effective date here. Its rules bring its duties into force in stages, and
  the dates were not confirmed. The layer applies the Act's duties now.
- Every advertising mail carries an objection route, though s. 6(4) asks only that withdrawal is
  easy.

## Review

This layer is a draft. It states no `reviewed` or `reviewer`, so no citation is shown to a rep until
a legal review stamps it.
