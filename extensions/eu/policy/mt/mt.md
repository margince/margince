---
jurisdiction: mt
version: 1
created: 2026-11-19
instruments:
  - name: Directive 2002/58/EC
    effective: 2002-07-31
    provision: "Art. 13"
    source-url: https://eur-lex.europa.eu/eli/dir/2002/58/oj
  - name: "Processing of Personal Data (Electronic Communications Sector) Regulations, S.L. 586.01"
    provision: "regulation 9"
fixtures:
  - finding: objection_route
    triggers: mt/fixtures/advertising-without-objection-route.md
    clears: mt/fixtures/advertising-with-objection-route.md
---

# What Maltese law requires of an outbound message

This layer sits on `regional/eu.md` and may only tighten it. It rests on regulation 9 of the
Processing of Personal Data (Electronic Communications Sector) Regulations, S.L. 586.01.

## Advertising to a company

The national rule covers legal persons as well as natural persons, so advertising to a company
address needs consent too. That is what the EU layer already applies.

## The existing-customer exception

The national act keeps the four conditions of the EU layer, so this layer inherits the exception and
adds no key for it.

## Disclosures

The EU layer's disclosures apply.

## Where this layer takes the stricter reading

- Whether regulation 9 binds advertising to legal persons was not checked; consent is asked for
  every address.

## Review

This layer is a draft. It states no `reviewed` or `reviewer`, so no citation is shown to a rep until
a legal review stamps it.
