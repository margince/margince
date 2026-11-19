---
jurisdiction: at
version: 1
created: 2026-11-19
instruments:
  - name: Directive 2002/58/EC
    effective: 2002-07-31
    provision: "Art. 13"
    source-url: https://eur-lex.europa.eu/eli/dir/2002/58/oj
  - name: "Telekommunikationsgesetz 2021 (TKG 2021)"
    provision: "§ 174"
  - name: "E-Commerce-Gesetz (ECG)"
    provision: "§ 7"
marketing-exceptions: []
fixtures:
  - finding: objection_route
    triggers: at/fixtures/advertising-without-objection-route.md
    clears: at/fixtures/advertising-with-objection-route.md
---

# What Austrian law requires of an outbound message

This layer sits on `regional/eu.md` and may only tighten it. It rests on § 174 of the
Telekommunikationsgesetz 2021 (TKG 2021). The E-Commerce-Gesetz (ECG) § 7 adds the list of addresses
whose holders refuse advertising, kept by the regulator RTR.

## Advertising to a company

§ 174(3) TKG 2021 asks for prior consent from every recipient, consumer or company. That is what the
EU layer already applies.

## The existing-customer exception

This layer states no exception. § 174(4) TKG 2021 adds a fifth condition to the four of the EU
layer: the recipient is not on the ECG list of § 7(2) ECG. The engine does not read that list, so it
cannot check the condition. An exception without it would send where Austrian law refuses.

## Disclosures

The EU layer's disclosures apply. § 174(4) also asks that every message offers a free and easy
refusal, which the objection route carries.

## Where this layer takes the stricter reading

- The existing-customer exception is left out, because the ECG list condition cannot be checked.

## Review

This layer is a draft. It states no `reviewed` or `reviewer`, so no citation is shown to a rep until
a legal review stamps it.
