---
jurisdiction: lv
version: 1
created: 2026-11-19
source-checked: 2026-11-25
instruments:
  - name: "Informācijas sabiedrības pakalpojumu likums"
    provision: "9. pants"
    source-url: https://likumi.lv/ta/id/96619
    text-digest: e2ae40eb25607cbfc352954b22fbf0db1ec64cbe4d78ef429d2e2628ed5dbc07
    text-fetched: 2026-11-25
fixtures:
  - finding: objection_route
    triggers: lv/fixtures/advertising-without-objection-route.md
    clears: lv/fixtures/advertising-with-objection-route.md
citations:
  - code: no_marketing_consent
    territorial-test: any_sender
    instrument: "Informācijas sabiedrības pakalpojumu likums"
    provision: "9. panta pirmā daļa"
  - code: marketing_permission_required
    territorial-test: any_sender
    instrument: "Informācijas sabiedrības pakalpojumu likums"
    provision: "9. panta pirmā daļa"
  - code: marketing_consent_required
    territorial-test: any_sender
    instrument: "Informācijas sabiedrības pakalpojumu likums"
    provision: "9. panta pirmā daļa"
  - code: unconfirmed_double_opt_in
    territorial-test: any_sender
    instrument: "Informācijas sabiedrības pakalpojumu likums"
    provision: "9. panta pirmā daļa"
  - code: public_opt_out_link
    territorial-test: any_sender
    instrument: "Informācijas sabiedrības pakalpojumu likums"
    provision: "9. panta ceturtā daļa"
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
