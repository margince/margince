---
jurisdiction: cn
version: 1
created: 2026-11-19
instruments:
  - name: "Measures for the Administration of Internet E-mail Services (MII Order No. 38)"
    effective: 2006-03-30
    provision: "arts. 13 and 14"
  - name: Advertising Law of the People's Republic of China
    effective: 2015-09-01
    provision: "art. 43"
  - name: Personal Information Protection Law of the People's Republic of China
    effective: 2021-11-01
    provision: "arts. 13 and 17"
reply-window: 8760h
deal-follow-up-window: 4368h
marketing-exceptions: []
consent-only: true
disclosures:
  - kind: controller_identity
  - kind: privacy_contact
  - kind: objection_route
    marketing-only: true
    means: [email]
  - kind: advertiser_contact
    marketing-only: true
subject-prefix: "广告"
no-legitimate-interests: true
fixtures:
  - finding: objection_route
    triggers: cn/fixtures/advertising-without-objection-route.md
    clears: cn/fixtures/advertising-complete.md
  - finding: objection_route
    triggers: cn/fixtures/advertising-refusal-by-web-only.md
    clears: cn/fixtures/advertising-complete.md
  - finding: advertiser_contact
    triggers: cn/fixtures/advertising-without-advertiser-contact.md
    clears: cn/fixtures/advertising-complete.md
  - finding: subject_prefix
    triggers: cn/fixtures/advertising-without-label.md
    clears: cn/fixtures/advertising-complete.md
  - finding: controller_identity
    triggers: cn/fixtures/notice-without-postal-address.md
    clears: cn/fixtures/notice-complete.md
  - finding: privacy_contact
    triggers: cn/fixtures/notice-without-privacy-contact.md
    clears: cn/fixtures/notice-complete.md
---

# What Chinese law requires of an outbound message

This layer rests on the Measures for the Administration of Internet E-mail Services
(互联网电子邮件服务管理办法, 2006), art. 43 of the Advertising Law, and the Personal Information
Protection Law (PIPL).

## Advertising needs express consent

Art. 13 of the Measures forbids commercial advertising email without the recipient's express
consent. Art. 43 of the Advertising Law asks for consent or the recipient's own request before any
advertising by electronic means. `consent-only` adds that no one-message exception a sender or an
admin records takes the place of consent. Neither instrument grants an exception.

## The subject label (Measures art. 13)

An advertising subject starts with 广告 or AD. The engine checks one prefix, so `subject-prefix` is
广告.

## Disclosures

- Advertising Law art. 43: advertising states the sender's true identity and contact details
  (`advertiser_contact`), and gives a way to refuse more of it.
- Measures art. 14: the way to refuse includes the sender's email address, and stays valid for 30
  days. So the objection route must name an email address.
- PIPL art. 17: before processing, the processor states its name and contact details. A Chinese
  controller carries `controller_identity` and `privacy_contact` on every first message.

## No legitimate interests (PIPL art. 13)

PIPL lists the grounds for processing, and a legitimate interest is not among them. So
`no-legitimate-interests` is set: each use rests on consent or on a ground the law names.

## What this layer does not state

- PIPL art. 3 reaches a processor abroad that serves people in China. The pack does not set
  `data-protection-follows-recipient`. That key would also move request deadlines and the privacy
  page to this pack, which states neither. It is a question for review.
- Advertising Law art. 9 bans words such as 最佳 and 国家级. The `claims-need-proof` key flags a
  claim that a proof clears, and no proof clears a banned word, so the ban is not stated.

## Where this layer takes the stricter reading

- A subject that starts with AD alone is held, though the Measures accept it.
- The objection route must name an email address in every advertising mail, though art. 14 asks
  for a working one only.

## Review

This layer is a draft. It states no `reviewed` or `reviewer`, so no citation is shown to a rep until
a legal review stamps it.
