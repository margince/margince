---
jurisdiction: gr
version: 1
created: 2026-11-19
instruments:
  - name: Directive 2002/58/EC
    effective: 2002-07-31
    provision: "Art. 13"
    source-url: https://eur-lex.europa.eu/eli/dir/2002/58/oj
  - name: "Νόμος 3471/2006"
    provision: "άρθρο 11"
fixtures:
  - finding: objection_route
    triggers: gr/fixtures/advertising-without-objection-route.md
    clears: gr/fixtures/advertising-with-objection-route.md
---

# What Greek law requires of an outbound message

This layer sits on `regional/eu.md` and may only tighten it. It rests on article 11 of Law 3471/2006
on the protection of personal data in electronic communications.

## Advertising to a company

Article 11 asks for consent from every recipient, a natural or a legal person. That is what the EU
layer already applies.

## The existing-customer exception

Article 11(3) keeps the four conditions of the EU layer. This layer inherits the exception and adds
no key for it.

## Disclosures

The EU layer's disclosures apply.

## Review

This layer is a draft. It states no `reviewed` or `reviewer`, so no citation is shown to a rep until
a legal review stamps it.
