---
jurisdiction: cz
version: 1
created: 2026-11-19
instruments:
  - name: Directive 2002/58/EC
    effective: 2002-07-31
    provision: "Art. 13"
    source-url: https://eur-lex.europa.eu/eli/dir/2002/58/oj
  - name: "Zákon č. 480/2004 Sb., o některých službách informační společnosti"
    provision: "§ 7"
fixtures:
  - finding: objection_route
    triggers: cz/fixtures/advertising-without-objection-route.md
    clears: cz/fixtures/advertising-with-objection-route.md
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
