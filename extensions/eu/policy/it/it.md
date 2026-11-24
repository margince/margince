---
jurisdiction: it
version: 1
created: 2026-11-19
instruments:
  - name: "Codice in materia di protezione dei dati personali (d.lgs. 30 giugno 2003, n. 196)"
    provision: "art. 130"
    source-url: https://www.normattiva.it/uri-res/N2Ls?urn:nir:stato:decreto.legislativo:2003-06-30;196~art130
consent-confirmation-needs-request: true
fixtures:
  - finding: objection_route
    triggers: it/fixtures/advertising-without-objection-route.md
    clears: it/fixtures/advertising-with-objection-route.md
citations:
  - code: no_marketing_consent
    territorial-test: any_sender
    instrument: "Codice in materia di protezione dei dati personali (d.lgs. 30 giugno 2003, n. 196)"
    provision: "art. 130, commi 1 e 2"
  - code: marketing_permission_required
    territorial-test: any_sender
    instrument: "Codice in materia di protezione dei dati personali (d.lgs. 30 giugno 2003, n. 196)"
    provision: "art. 130, commi 1 e 2"
  - code: marketing_consent_required
    territorial-test: any_sender
    instrument: "Codice in materia di protezione dei dati personali (d.lgs. 30 giugno 2003, n. 196)"
    provision: "art. 130, commi 1 e 2"
  - code: unconfirmed_double_opt_in
    territorial-test: any_sender
    instrument: "Codice in materia di protezione dei dati personali (d.lgs. 30 giugno 2003, n. 196)"
    provision: "art. 130, commi 1 e 2"
  - code: public_opt_out_link
    territorial-test: any_sender
    instrument: "Codice in materia di protezione dei dati personali (d.lgs. 30 giugno 2003, n. 196)"
    provision: "art. 130, comma 5"
---

# What Italian law requires of an outbound message

This layer sits on `regional/eu.md` and may only tighten it. It rests on art. 130 of the Codice in
materia di protezione dei dati personali (d.lgs. 196/2003). The Garante's guidelines on promotional
activity and spam of 4 July 2013 read it.

## Advertising to a company

Art. 130 protects the contraente as well as the user, which includes a legal person. So advertising
to a company address needs consent too, as the EU layer already applies.

## The existing-customer exception

Art. 130(4) keeps the four conditions of the EU layer. This layer inherits the exception and adds no
key for it.

## Disclosures

The EU layer's disclosures apply. Art. 130(5) also forbids hiding the sender and asks for a valid
address to object.

## Where this layer takes the stricter reading

- The Garante treats an email that asks for consent to advertising as advertising itself. So
  `consent-confirmation-needs-request` is true: a mail asking for consent goes out only when the
  recipient's own request is on record.

## Review

This layer is a draft. It states no `reviewed` or `reviewer`, so no citation is shown to a rep until
a legal review stamps it.
