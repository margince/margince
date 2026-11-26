---
jurisdiction: se
version: 1
created: 2026-11-19
source-checked: 2026-11-25
instruments:
  - name: "Marknadsföringslagen (2008:486)"
    provision: "19–20 §§"
    source-url: https://www.riksdagen.se/sv/dokument-och-lagar/dokument/svensk-forfattningssamling/marknadsforingslag-2008486_sfs-2008-486/
    text-digest: c498cd9922bd6168aa8b05d9f31bff8ac23e6f62caec78a31c2ea9cd0b882421
    text-fetched: 2026-11-25
fixtures:
  - finding: objection_route
    triggers: se/fixtures/advertising-without-objection-route.md
    clears: se/fixtures/advertising-with-objection-route.md
citations:
  - code: no_marketing_consent
    territorial-test: any_sender
    instrument: "Marknadsföringslagen (2008:486)"
    provision: "19 § första stycket"
  - code: marketing_permission_required
    territorial-test: any_sender
    instrument: "Marknadsföringslagen (2008:486)"
    provision: "19 § första stycket"
  - code: marketing_consent_required
    territorial-test: any_sender
    instrument: "Marknadsföringslagen (2008:486)"
    provision: "19 § första stycket"
  - code: unconfirmed_double_opt_in
    territorial-test: any_sender
    instrument: "Marknadsföringslagen (2008:486)"
    provision: "19 § första stycket"
  - code: public_opt_out_link
    territorial-test: any_sender
    instrument: "Marknadsföringslagen (2008:486)"
    provision: "20 §"
---

# What Swedish law requires of an outbound message

This layer sits on `regional/eu.md` and may only tighten it. It rests on sections 19 and 20 of the
Swedish Marketing Act (marknadsföringslagen, 2008:486).

## Advertising to a company

Section 19 binds advertising to natural persons; section 20 asks only for a valid address to object
when the recipient is a legal person. The engine cannot tell a company's general address from an
individual's own address, so this pack asks for consent for every address. That is stricter than the
law, and the core already applies it.

## The existing-customer exception

Section 19, second paragraph, keeps the four conditions of the EU layer. This layer inherits the
exception and adds no key for it.

## Disclosures

The EU layer's disclosures apply. Section 20 also asks for a valid address to which the recipient
can send a request to stop.

## Where this layer takes the stricter reading

- Advertising to a legal person still needs consent here.

## Review

This layer is a draft. It states no `reviewed` or `reviewer`, so no citation is shown to a rep until
a legal review stamps it.
