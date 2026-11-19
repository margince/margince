---
jurisdiction: fr
version: 1
created: 2026-11-19
instruments:
  - name: Directive 2002/58/EC
    effective: 2002-07-31
    provision: "Art. 13"
    source-url: https://eur-lex.europa.eu/eli/dir/2002/58/oj
  - name: "Code des postes et des communications électroniques"
    provision: "art. L34-5"
  - name: "Loi n° 2004-575 du 21 juin 2004 pour la confiance dans l'économie numérique"
    provision: "art. 20"
fixtures:
  - finding: objection_route
    triggers: fr/fixtures/advertising-without-objection-route.md
    clears: fr/fixtures/advertising-with-objection-route.md
---

# What French law requires of an outbound message

This layer sits on `regional/eu.md` and may only tighten it. It rests on art. L34-5 of the Code des
postes et des communications électroniques. Art. 20 of the loi pour la confiance dans l'économie
numérique (LCEN) asks that advertising is recognisable as such and names who it is for.

## Advertising to a company

L34-5 binds advertising to natural persons. The CNIL accepts advertising to a professional address
without consent when it concerns the recipient's profession. The engine cannot tell a company's
general address from an individual's own address, so this pack asks for consent for every address. That
is stricter than the law, and the core already applies it.

## The existing-customer exception

L34-5 keeps the four conditions of the EU layer. This layer inherits the exception and adds no key
for it.

## Disclosures

The EU layer's disclosures apply. L34-5 also forbids a subject that has nothing to do with the
product or service offered; the engine does not check the subject's meaning.

## Where this layer takes the stricter reading

- Advertising to a professional address still needs consent here.

## Review

This layer is a draft. It states no `reviewed` or `reviewer`, so no citation is shown to a rep until
a legal review stamps it.
