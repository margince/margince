---
jurisdiction: vn
version: 5
created: 2026-09-04
instruments:
  - name: Decree 91/2020/ND-CP
    effective: 2020-10-01
  - name: Law 91/2025/QH15
    effective: 2026-01-01
  - name: Decree 356/2025/ND-CP
    effective: 2026-01-01
reply-window: 8760h
deal-follow-up-window: 4368h
marketing-exceptions: []
consent-only: true
disclosures:
  - kind: controller_identity
  - kind: privacy_contact
  - kind: objection_route
    marketing-only: true
    means: [web, email, phone]
  - kind: advertiser_contact
    marketing-only: true
refusal-section-last: true
claims-need-proof:
  - nhất
  - duy nhất
  - tốt nhất
  - số một
  - số 1
  - hàng đầu
  - best
  - no. 1
  - number one
  - leading
  - the only
  - unique
foreign-advertiser-needs-local-agency: true
subject-prefix: "[QC]"
frequency-cap:
  messages: 3
  window: 24h
opt-out-acknowledgement: true
opt-out-acknowledgement-within: 1h
opt-out-acknowledgement-utc-offset: 7h
opt-out-records-keep: 8784h
data-protection-follows-recipient: true
disclosure-language: vi
no-legitimate-interests: true
consent-confirmation-needs-request: true
consent-request-first-for: [marketing, phone_outreach]
fixtures:
  - finding: claim_without_proof
    triggers: fixtures/claim-without-proof.md
    clears: fixtures/claim-with-proof.md
  - finding: claim_without_proof
    triggers: fixtures/number-one-without-proof.md
    clears: fixtures/number-one-with-proof.md
  - finding: advertising_language_first
    triggers: fixtures/english-first.md
    clears: fixtures/claim-with-proof.md
  - finding: foreign_advertiser
    triggers: fixtures/sent-from-abroad.md
    clears: fixtures/claim-with-proof.md
  - finding: subject_prefix
    triggers: fixtures/unlabelled-subject.md
    clears: fixtures/claim-with-proof.md
  - finding: objection_route
    triggers: fixtures/refusal-without-phone.md
    clears: fixtures/claim-with-proof.md
  - finding: advertiser_contact
    triggers: fixtures/no-advertiser-contact.md
    clears: fixtures/claim-with-proof.md
  - finding: privacy_contact
    triggers: fixtures/invoice-from-abroad-without-privacy-contact.md
    clears: fixtures/invoice-from-abroad.md
---

# What Vietnamese law requires of an outbound message

This layer states the rules as data. The core engine applies them, and nothing here decides a send.

## The instruments

Decree 91/2020/ND-CP is the source of every advertising-email duty below: the daily ceiling, the
`[QC]` label, the advertiser's identity and the acknowledged opt-out.

Law 91/2025/QH15 and Decree 356/2025/ND-CP govern personal data protection. Both took effect on
1 January 2026, and neither amends the 2020 decree. They are listed because a Vietnamese recipient
of advertising email is a data subject under them. Whether they add an outbound-messaging duty this
layer does not state is a question for a Vietnamese lawyer.

The instruments are stated for the reader of a decision. The engine applies the keys below, never
an instrument.

## The version

Version 2 stated the 2025 instruments. Version 3 applied them: the data-protection rules follow the
recipient, no use rests on legitimate interests, and the receipt is due at once in Vietnam time.
Version 4 added that a consent mail answers the recipient's own request. Version 5 asks for three
refusal means, ends advertising with the refusal section, and flags claims that need a proof.

## No marketing exception

Decree 91/2020/ND-CP Art. 10 allows advertising email only with the recipient's prior consent. It
grants no route that a sale opens, so `marketing-exceptions` is stated and empty. Evidence of a
German sale authorizes nothing here. `consent-only` adds that no exception a sender or an admin
records takes the place of consent.

## Disclosures

- Art. 19: an advertising message names the advertiser and a way to reach them
  (`advertiser_contact`).
- Art. 20.2: the recipient can refuse by web, email and phone, so the objection route names all
  three means.
- GDPR-style controller duties (`controller_identity`, `privacy_contact`) apply to every message.

Advertising ends with the refusal section (`refusal-section-last`) and is written in Vietnamese
first (`disclosure-language`).

## Content

Law on Advertising Art. 8.11 and Circular 12/2026/TT-BVHTTDL Art. 3 allow a claim of a leading,
unique or absolute position only with a listed proof in the mail. Art. 3.1 reaches such a claim in
any language, so the English forms are listed too. "nhất" alone also catches the forms built on it.
The list has "the only" and not "only", because a bare "only" limits far more often than it ranks.

Law on Advertising Art. 39.2: a foreign advertiser goes through a Vietnamese advertising business.
The customer is the advertiser.

## Label, ceiling and receipt

- Art. 18: an advertising subject carries the label `[QC]`.
- Art. 13.5: at most three advertising messages to one address in 24 hours. The engine counts
  messages the recipient received.
- Art. 20: a refusal is acknowledged at once. The receipt states when the refusal arrived and when
  sending stopped, in Vietnam time (UTC+7). One hour is when an unsent receipt raises an alarm.
- Art. 11.7: registrations, refusals and receipts are kept at least one year. 8784 hours is 366
  days, which holds across a leap year.

## Consent requests

Decree 91/2020/ND-CP Art. 11.2: a consent request the recipient did not ask for is itself an
unsolicited message. It governs advertising, so only the advertising purpose classes need the
request first. Law 91/2025/QH15 Art. 9 and 11.1 need consent before collection, and the law has no
legitimate-interest basis. Art. 1.2(c) reaches a foreign controller that processes a Vietnamese
citizen's data, so the data-protection duties follow the recipient.
