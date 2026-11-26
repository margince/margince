---
jurisdiction: pt
version: 1
created: 2026-11-19
instruments:
  - name: "Lei n.º 41/2004, de 18 de agosto"
    provision: "art. 13.º-A"
    source-url: https://diariodarepublica.pt/dr/legislacao-consolidada/lei/2004-106523049
  - name: "Decreto-Lei n.º 7/2004, de 7 de janeiro"
    provision: "art. 22.º"
fixtures:
  - finding: objection_route
    triggers: pt/fixtures/advertising-without-objection-route.md
    clears: pt/fixtures/advertising-with-objection-route.md
citations:
  - code: no_marketing_consent
    territorial-test: any_sender
    instrument: "Lei n.º 41/2004, de 18 de agosto"
    provision: "art. 13.º-A, n.º 1"
  - code: marketing_permission_required
    territorial-test: any_sender
    instrument: "Lei n.º 41/2004, de 18 de agosto"
    provision: "art. 13.º-A, n.º 1"
  - code: marketing_consent_required
    territorial-test: any_sender
    instrument: "Lei n.º 41/2004, de 18 de agosto"
    provision: "art. 13.º-A, n.º 1"
  - code: unconfirmed_double_opt_in
    territorial-test: any_sender
    instrument: "Lei n.º 41/2004, de 18 de agosto"
    provision: "art. 13.º-A, n.º 1"
  - code: public_opt_out_link
    territorial-test: any_sender
    instrument: "Lei n.º 41/2004, de 18 de agosto"
    provision: "art. 13.º-A, n.º 4"
---

# What Portuguese law requires of an outbound message

This layer sits on `regional/eu.md` and may only tighten it. It rests on art. 13.º-A of Lei n.º
41/2004 and art. 22.º of Decreto-Lei n.º 7/2004 on electronic commerce.

## Advertising to a company

Art. 13.º-B lets a sender advertise to a legal person unless it has refused, and keeps a national
list of those who refuse. The engine cannot tell a company's general address from an individual's own
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
