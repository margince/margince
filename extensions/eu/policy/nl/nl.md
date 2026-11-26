---
jurisdiction: nl
version: 1
created: 2026-11-19
source-checked: 2026-11-25
instruments:
  - name: "Telecommunicatiewet"
    provision: "art. 11.7"
    source-url: https://wetten.overheid.nl/BWBR0009950/
    text-digest: 99c230aab81c690af9fa2875cf42fd4748eb8fa664d7ee56a50198f3a29486b6
    text-fetched: 2026-11-25
fixtures:
  - finding: objection_route
    triggers: nl/fixtures/advertising-without-objection-route.md
    clears: nl/fixtures/advertising-with-objection-route.md
citations:
  - code: no_marketing_consent
    territorial-test: any_sender
    instrument: "Telecommunicatiewet"
    provision: "art. 11.7, eerste lid"
  - code: marketing_permission_required
    territorial-test: any_sender
    instrument: "Telecommunicatiewet"
    provision: "art. 11.7, eerste lid"
  - code: marketing_consent_required
    territorial-test: any_sender
    instrument: "Telecommunicatiewet"
    provision: "art. 11.7, eerste lid"
  - code: unconfirmed_double_opt_in
    territorial-test: any_sender
    instrument: "Telecommunicatiewet"
    provision: "art. 11.7, eerste lid"
  - code: public_opt_out_link
    territorial-test: any_sender
    instrument: "Telecommunicatiewet"
    provision: "art. 11.7, zevende lid"
---

# What Dutch law requires of an outbound message

This layer sits on `regional/eu.md` and may only tighten it. It rests on art. 11.7 of the
Telecommunicatiewet.

## Advertising to a company

Art. 11.7 sets a lighter rule for an address a legal person has published for contact. The engine
cannot tell a company's general address from an individual's own address, so this pack asks for consent
for every address. That is stricter than the law, and the core already applies it.

## The existing-customer exception

Art. 11.7(3) keeps the four conditions of the EU layer. This layer inherits the exception and adds
no key for it.

## Disclosures

The EU layer's disclosures apply. Art. 11.7(4) also asks that the message shows who sends it and
gives a valid address to object.

## Where this layer takes the stricter reading

- Advertising to a published company address still needs consent here.

## Review

This layer is a draft. It states no `reviewed` or `reviewer`, so no citation is shown to a rep until
a legal review stamps it.
