---
jurisdiction: pt
version: 1
created: 2026-11-19
instruments:
  - name: Directive 2002/58/EC
    effective: 2002-07-31
    provision: "Art. 13"
    source-url: https://eur-lex.europa.eu/eli/dir/2002/58/oj
  - name: "Lei n.º 41/2004, de 18 de agosto"
    provision: "art. 13.º-A"
  - name: "Decreto-Lei n.º 7/2004, de 7 de janeiro"
    provision: "art. 22.º"
fixtures:
  - finding: objection_route
    triggers: pt/fixtures/advertising-without-objection-route.md
    clears: pt/fixtures/advertising-with-objection-route.md
---

# What Portuguese law requires of an outbound message

This layer sits on `regional/eu.md` and may only tighten it. It rests on art. 13.º-A of Lei n.º
41/2004 and art. 22.º of Decreto-Lei n.º 7/2004 on electronic commerce.

## Advertising to a company

Art. 13.º-B lets a sender advertise to a legal person unless it has refused, and keeps a national
list of those who refuse. The engine cannot tell a company's general address from a person's own
address, so this pack asks for consent for every address. That is stricter than the law, and the
core already applies it.

## The existing-customer exception

Art. 13.º-A(3) keeps the four conditions of the EU layer. This layer inherits the exception and adds
no key for it.

## Disclosures

The EU layer's disclosures apply.

## Where this layer takes the stricter reading

- Advertising to a legal person still needs consent here.

## Review

This layer is a draft. It states no `reviewed` or `reviewer`, so no citation is shown to a rep until
a legal review stamps it.
