---
jurisdiction: dk
version: 1
created: 2026-11-19
instruments:
  - name: "Markedsføringsloven"
    provision: "§ 10"
    source-url: https://www.retsinformation.dk/eli/lta/2022/866
fixtures:
  - finding: objection_route
    triggers: dk/fixtures/advertising-without-objection-route.md
    clears: dk/fixtures/advertising-with-objection-route.md
citations:
  - code: no_marketing_consent
    territorial-test: any_sender
    instrument: "Markedsføringsloven"
    provision: "§ 10, stk. 1"
  - code: marketing_permission_required
    territorial-test: any_sender
    instrument: "Markedsføringsloven"
    provision: "§ 10, stk. 1"
  - code: marketing_consent_required
    territorial-test: any_sender
    instrument: "Markedsføringsloven"
    provision: "§ 10, stk. 1"
  - code: unconfirmed_double_opt_in
    territorial-test: any_sender
    instrument: "Markedsføringsloven"
    provision: "§ 10, stk. 1"
  - code: public_opt_out_link
    territorial-test: any_sender
    instrument: "Markedsføringsloven"
    provision: "§ 10, stk. 3"
---

# What Danish law requires of an outbound message

This layer sits on `regional/eu.md` and may only tighten it. It rests on § 10 of the Danish
Marketing Practices Act (markedsføringsloven).

## Advertising to a company

§ 10 covers every recipient, a consumer or a company, so advertising to a company address needs
consent too. That is what the EU layer already applies.

## The existing-customer exception

§ 10(2) keeps the four conditions of the EU layer. This layer inherits the exception and adds no key
for it.

## Disclosures

The EU layer's disclosures apply.

## Review

This layer is a draft. It states no `reviewed` or `reviewer`, so no citation is shown to a rep until
a legal review stamps it.
