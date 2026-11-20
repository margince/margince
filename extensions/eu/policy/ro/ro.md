---
jurisdiction: ro
version: 1
created: 2026-11-19
instruments:
  - name: "Legea nr. 506/2004 privind prelucrarea datelor cu caracter personal și protecția vieții private în sectorul comunicațiilor electronice"
    provision: "art. 12"
    source-url: https://legislatie.just.ro/Public/DetaliiDocument/56973
fixtures:
  - finding: objection_route
    triggers: ro/fixtures/advertising-without-objection-route.md
    clears: ro/fixtures/advertising-with-objection-route.md
citations:
  - code: no_marketing_consent
    territorial-test: any_sender
    instrument: "Legea nr. 506/2004 privind prelucrarea datelor cu caracter personal și protecția vieții private în sectorul comunicațiilor electronice"
    provision: "art. 12 alin. (1)"
  - code: marketing_permission_required
    territorial-test: any_sender
    instrument: "Legea nr. 506/2004 privind prelucrarea datelor cu caracter personal și protecția vieții private în sectorul comunicațiilor electronice"
    provision: "art. 12 alin. (1)"
  - code: marketing_consent_required
    territorial-test: any_sender
    instrument: "Legea nr. 506/2004 privind prelucrarea datelor cu caracter personal și protecția vieții private în sectorul comunicațiilor electronice"
    provision: "art. 12 alin. (1)"
  - code: unconfirmed_double_opt_in
    territorial-test: any_sender
    instrument: "Legea nr. 506/2004 privind prelucrarea datelor cu caracter personal și protecția vieții private în sectorul comunicațiilor electronice"
    provision: "art. 12 alin. (1)"
  - code: public_opt_out_link
    territorial-test: any_sender
    instrument: "Legea nr. 506/2004 privind prelucrarea datelor cu caracter personal și protecția vieții private în sectorul comunicațiilor electronice"
    provision: "art. 12 alin. (3)"
---

# What Romanian law requires of an outbound message

This layer sits on `regional/eu.md` and may only tighten it. It rests on art. 12 of Law no. 506/2004
on personal data and privacy in electronic communications.

## Advertising to a company

The national rule covers legal persons as well as natural persons, so advertising to a company
address needs consent too. That is what the EU layer already applies.

## The existing-customer exception

Art. 12(2) keeps the four conditions of the EU layer. This layer inherits the exception and adds no
key for it.

## Disclosures

The EU layer's disclosures apply.

## Where this layer takes the stricter reading

- Whether art. 12 binds advertising to legal persons was not checked; consent is asked for every
  address.

## Review

This layer is a draft. It states no `reviewed` or `reviewer`, so no citation is shown to a rep until
a legal review stamps it.
