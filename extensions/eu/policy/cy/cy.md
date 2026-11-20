---
jurisdiction: cy
version: 1
created: 2026-11-19
instruments:
  - name: "Ο περί Ρυθμίσεως Ηλεκτρονικών Επικοινωνιών και Ταχυδρομικών Υπηρεσιών Νόμος 112(Ι)/2004"
    provision: "άρθρο 106"
fixtures:
  - finding: objection_route
    triggers: cy/fixtures/advertising-without-objection-route.md
    clears: cy/fixtures/advertising-with-objection-route.md
citations:
  - code: no_marketing_consent
    territorial-test: any_sender
    instrument: "Directive 2002/58/EC"
    provision: "Art. 13(1)"
  - code: marketing_permission_required
    territorial-test: any_sender
    instrument: "Directive 2002/58/EC"
    provision: "Art. 13(1)"
  - code: marketing_consent_required
    territorial-test: any_sender
    instrument: "Directive 2002/58/EC"
    provision: "Art. 13(1)"
  - code: unconfirmed_double_opt_in
    territorial-test: any_sender
    instrument: "Directive 2002/58/EC"
    provision: "Art. 13(1)"
  - code: public_opt_out_link
    territorial-test: any_sender
    instrument: "Directive 2002/58/EC"
    provision: "Art. 13(4)"
---

# What Cypriot law requires of an outbound message

This layer sits on `regional/eu.md` and may only tighten it. It rests on section 106 of Law
112(I)/2004 on the regulation of electronic communications and postal services, which carries Art.
13 into Cypriot law.

## Advertising to a company

The national rule covers legal persons as well as natural persons, so advertising to a company
address needs consent too. That is what the EU layer already applies.

## The existing-customer exception

The national act keeps the four conditions of the EU layer, so this layer inherits the exception and
adds no key for it.

## Disclosures

The EU layer's disclosures apply.

## Where this layer takes the stricter reading

- Cyprus has since passed a newer electronic communications law. The review must confirm where
  section 106 now sits.

## Review

This layer is a draft. It states no `reviewed` or `reviewer`, so no citation is shown to a rep until
a legal review stamps it.
