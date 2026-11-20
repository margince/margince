---
jurisdiction: lv
version: 1
created: 2026-11-19
instruments:
  - name: Directive 2002/58/EC
    effective: 2002-07-31
    provision: "Art. 13"
    source-url: https://eur-lex.europa.eu/eli/dir/2002/58/oj
  - name: "Informācijas sabiedrības pakalpojumu likums"
    provision: "9. pants"
fixtures:
  - finding: objection_route
    triggers: lv/fixtures/advertising-without-objection-route.md
    clears: lv/fixtures/advertising-with-objection-route.md
---

# What Latvian law requires of an outbound message

This layer sits on `regional/eu.md` and may only tighten it. It rests on article 9 of the
Information Society Services Law (Informācijas sabiedrības pakalpojumu likums).

## Advertising to a company

The national rule covers legal persons as well as natural persons, so advertising to a company
address needs consent too. That is what the EU layer already applies.

## The existing-customer exception

The national act keeps the four conditions of the EU layer, so this layer inherits the exception and
adds no key for it.

## Disclosures

The EU layer's disclosures apply.

## Where this layer takes the stricter reading

- Whether article 9 binds advertising to legal persons was not checked; consent is asked for every
  address.

## Review

This layer is a draft. It states no `reviewed` or `reviewer`, so no citation is shown to a rep until
a legal review stamps it.
