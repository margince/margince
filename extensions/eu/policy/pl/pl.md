---
jurisdiction: pl
version: 1
created: 2026-11-19
instruments:
  - name: "Ustawa z dnia 12 lipca 2024 r. Prawo komunikacji elektronicznej"
    provision: "art. 398"
    source-url: https://eli.gov.pl/eli/DU/2024/1221/ogl
marketing-exceptions: []
fixtures:
  - finding: objection_route
    triggers: pl/fixtures/advertising-without-objection-route.md
    clears: pl/fixtures/advertising-with-objection-route.md
citations:
  - code: no_marketing_consent
    territorial-test: any_sender
    instrument: "Ustawa z dnia 12 lipca 2024 r. Prawo komunikacji elektronicznej"
    provision: "art. 398 ust. 1"
  - code: marketing_permission_required
    territorial-test: any_sender
    instrument: "Ustawa z dnia 12 lipca 2024 r. Prawo komunikacji elektronicznej"
    provision: "art. 398 ust. 1"
  - code: marketing_consent_required
    territorial-test: any_sender
    instrument: "Ustawa z dnia 12 lipca 2024 r. Prawo komunikacji elektronicznej"
    provision: "art. 398 ust. 1"
  - code: unconfirmed_double_opt_in
    territorial-test: any_sender
    instrument: "Ustawa z dnia 12 lipca 2024 r. Prawo komunikacji elektronicznej"
    provision: "art. 398 ust. 1"
  - code: public_opt_out_link
    territorial-test: any_sender
    instrument: "Directive 2002/58/EC"
    provision: "Art. 13(4)"
---

# What Polish law requires of an outbound message

This layer sits on `regional/eu.md` and may only tighten it. It rests on art. 398 of the Electronic
Communications Law of 12 July 2024 (Prawo komunikacji elektronicznej), which took over the rule from
the Telecommunications Law and the Act on providing services by electronic means.

## Advertising to a company

Art. 398 asks for prior consent from every subscriber or end user, a consumer or a company. That is
what the EU layer already applies.

## The existing-customer exception

This layer states no exception. Polish law, as this layer read it, names no existing-customer
exception for direct marketing by email, so consent is the only route.

## Disclosures

The EU layer's disclosures apply.

## Where this layer takes the stricter reading

- The existing-customer exception is left out. A review may find a route in the new law.

## Review

This layer is a draft. It states no `reviewed` or `reviewer`, so no citation is shown to a rep until
a legal review stamps it.
