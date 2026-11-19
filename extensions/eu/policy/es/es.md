---
jurisdiction: es
version: 1
created: 2026-11-19
instruments:
  - name: Directive 2002/58/EC
    effective: 2002-07-31
    provision: "Art. 13"
    source-url: https://eur-lex.europa.eu/eli/dir/2002/58/oj
  - name: "Ley 34/2002, de 11 de julio, de servicios de la sociedad de la información y de comercio electrónico (LSSI)"
    provision: "arts. 20 y 21"
subject-prefix: "Publicidad"
fixtures:
  - finding: objection_route
    triggers: es/fixtures/advertising-without-objection-route.md
    clears: es/fixtures/advertising-with-objection-route.md
  - finding: subject_prefix
    triggers: es/fixtures/advertising-without-prefix.md
    clears: es/fixtures/advertising-with-objection-route.md
---

# What Spanish law requires of an outbound message

This layer sits on `regional/eu.md` and may only tighten it. It rests on arts. 20 and 21 of Ley
34/2002 on information society services and electronic commerce (LSSI).

## Advertising to a company

Art. 21 protects every recipient, a natural or a legal person. So advertising to a company address
needs consent too, as the EU layer already applies.

## The existing-customer exception

Art. 21(2) keeps the four conditions of the EU layer. This layer inherits the exception and adds no
key for it.

## Disclosures

The EU layer's disclosures apply. Art. 20(1) also asks that an advertising email starts with the
word «publicidad» or the short form «publi». The engine checks one prefix on the subject, so
`subject-prefix` is «Publicidad».

## Where this layer takes the stricter reading

- A subject that starts with «publi» alone is held, though the law accepts it. One prefix is the
  only marking the engine checks, and the full word cannot be mistaken for another one.

## Review

This layer is a draft. It states no `reviewed` or `reviewer`, so no citation is shown to a rep until
a legal review stamps it.
