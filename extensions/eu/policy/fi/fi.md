---
jurisdiction: fi
version: 1
created: 2026-11-19
instruments:
  - name: Directive 2002/58/EC
    effective: 2002-07-31
    provision: "Art. 13"
    source-url: https://eur-lex.europa.eu/eli/dir/2002/58/oj
  - name: "Laki sähköisen viestinnän palveluista (917/2014)"
    provision: "200–202 §"
fixtures:
  - finding: objection_route
    triggers: fi/fixtures/advertising-without-objection-route.md
    clears: fi/fixtures/advertising-with-objection-route.md
---

# What Finnish law requires of an outbound message

This layer sits on `regional/eu.md` and may only tighten it. It rests on sections 200 to 202 of the
Act on Electronic Communications Services (laki sähköisen viestinnän palveluista, 917/2014).

## Advertising to a company

Section 201 lets a sender advertise to a legal person unless it has refused. The engine cannot tell
a company's general address from an individual's own address, so this pack asks for consent for every
address. That is stricter than the law, and the core already applies it.

## The existing-customer exception

Section 200(2) keeps the four conditions of the EU layer. This layer inherits the exception and adds
no key for it.

## Disclosures

The EU layer's disclosures apply. Section 202 also asks that the message shows it is advertising and
who sends it.

## Where this layer takes the stricter reading

- Advertising to a legal person still needs consent here.

## Review

This layer is a draft. It states no `reviewed` or `reviewer`, so no citation is shown to a rep until
a legal review stamps it.
