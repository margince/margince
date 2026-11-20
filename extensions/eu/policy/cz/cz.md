---
jurisdiction: cz
version: 1
created: 2026-11-19
instruments:
  - name: "Zákon č. 480/2004 Sb., o některých službách informační společnosti"
    provision: "§ 7"
    source-url: https://www.e-sbirka.cz/sb/2004/480
fixtures:
  - finding: objection_route
    triggers: cz/fixtures/advertising-without-objection-route.md
    clears: cz/fixtures/advertising-with-objection-route.md
citations:
  - code: no_marketing_consent
    territorial-test: any_sender
    instrument: "Zákon č. 480/2004 Sb., o některých službách informační společnosti"
    provision: "§ 7 odst. 2"
  - code: marketing_permission_required
    territorial-test: any_sender
    instrument: "Zákon č. 480/2004 Sb., o některých službách informační společnosti"
    provision: "§ 7 odst. 2"
  - code: marketing_consent_required
    territorial-test: any_sender
    instrument: "Zákon č. 480/2004 Sb., o některých službách informační společnosti"
    provision: "§ 7 odst. 2"
  - code: unconfirmed_double_opt_in
    territorial-test: any_sender
    instrument: "Zákon č. 480/2004 Sb., o některých službách informační společnosti"
    provision: "§ 7 odst. 2"
  - code: public_opt_out_link
    territorial-test: any_sender
    instrument: "Zákon č. 480/2004 Sb., o některých službách informační společnosti"
    provision: "§ 7 odst. 4 písm. b) a c)"
---

# What Czech law requires of an outbound message

This layer sits on `regional/eu.md` and may only tighten it. It rests on § 7 of Act No. 480/2004
Coll. on certain information society services (zákon č. 480/2004 Sb.).

## Advertising to a company

The national rule covers legal persons as well as natural persons, so advertising to a company
address needs consent too. That is what the EU layer already applies.

## The existing-customer exception

§ 7(3) keeps the four conditions of the EU layer. This layer inherits the exception and adds no key
for it.

## Disclosures

The EU layer's disclosures apply. § 7(4) also asks that a commercial message is clearly marked as
one. The law names no fixed word, so the engine has no marking to check.

## Review

This layer is a draft. It states no `reviewed` or `reviewer`, so no citation is shown to a rep until
a legal review stamps it.
