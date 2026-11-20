---
jurisdiction: lt
version: 1
created: 2026-11-19
instruments:
  - name: "Elektroninių ryšių įstatymas (Law on Electronic Communications)"
fixtures:
  - finding: objection_route
    triggers: lt/fixtures/advertising-without-objection-route.md
    clears: lt/fixtures/advertising-with-objection-route.md
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

# What Lithuanian law requires of an outbound message

This layer sits on `regional/eu.md` and may only tighten it. It rests on the Law on Electronic
Communications (Elektroninių ryšių įstatymas), whose article on direct marketing carries Art. 13
into Lithuanian law. The review must name the article.

## Advertising to a company

The national rule covers legal persons as well as natural persons, so advertising to a company
address needs consent too. That is what the EU layer already applies.

## The existing-customer exception

The national act keeps the four conditions of the EU layer, so this layer inherits the exception and
adds no key for it.

## Disclosures

The EU layer's disclosures apply.

## Where this layer takes the stricter reading

- The article number in the current consolidated text was not checked; the review must add it.

## Review

This layer is a draft. It states no `reviewed` or `reviewer`, so no citation is shown to a rep until
a legal review stamps it.
