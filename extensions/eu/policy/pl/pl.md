---
jurisdiction: pl
version: 1
created: 2026-11-19
instruments:
  - name: Directive 2002/58/EC
    effective: 2002-07-31
    provision: "Art. 13"
    source-url: https://eur-lex.europa.eu/eli/dir/2002/58/oj
  - name: "Ustawa z dnia 12 lipca 2024 r. Prawo komunikacji elektronicznej"
    provision: "art. 398"
marketing-exceptions: []
fixtures:
  - finding: objection_route
    triggers: pl/fixtures/advertising-without-objection-route.md
    clears: pl/fixtures/advertising-with-objection-route.md
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
