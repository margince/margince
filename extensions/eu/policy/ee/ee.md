---
jurisdiction: ee
version: 1
created: 2026-11-19
instruments:
  - name: "Elektroonilise side seadus (Electronic Communications Act)"
    provision: "§ 103¹"
    source-url: https://www.riigiteataja.ee/akt/ESS
  - name: "Infoühiskonna teenuse seadus (Information Society Services Act)"
    provision: "§ 6"
fixtures:
  - finding: objection_route
    triggers: ee/fixtures/advertising-without-objection-route.md
    clears: ee/fixtures/advertising-with-objection-route.md
citations:
  - code: no_marketing_consent
    territorial-test: any_sender
    instrument: "Elektroonilise side seadus (Electronic Communications Act)"
    provision: "§ 103¹ lg 1"
  - code: marketing_permission_required
    territorial-test: any_sender
    instrument: "Elektroonilise side seadus (Electronic Communications Act)"
    provision: "§ 103¹ lg 1"
  - code: marketing_consent_required
    territorial-test: any_sender
    instrument: "Elektroonilise side seadus (Electronic Communications Act)"
    provision: "§ 103¹ lg 1"
  - code: unconfirmed_double_opt_in
    territorial-test: any_sender
    instrument: "Elektroonilise side seadus (Electronic Communications Act)"
    provision: "§ 103¹ lg 1"
  - code: public_opt_out_link
    territorial-test: any_sender
    instrument: "Elektroonilise side seadus (Electronic Communications Act)"
    provision: "§ 103¹ lg 4"
---

# What Estonian law requires of an outbound message

This layer sits on `regional/eu.md` and may only tighten it. It rests on § 103¹ of the Electronic
Communications Act (elektroonilise side seadus) and § 6 of the Information Society Services Act
(infoühiskonna teenuse seadus).

## Advertising to a company

The Estonian rule for legal persons is narrower than for natural persons. The engine cannot tell a
company's general address from an individual's own address, so this pack asks for consent for every
address. That is stricter than the law, and the core already applies it.

## The existing-customer exception

The national act keeps the four conditions of the EU layer, so this layer inherits the exception and
adds no key for it.

## Disclosures

The EU layer's disclosures apply.

## Where this layer takes the stricter reading

- The rule for legal persons was not checked in detail; consent is asked for every address.

## Review

This layer is a draft. It states no `reviewed` or `reviewer`, so no citation is shown to a rep until
a legal review stamps it.
