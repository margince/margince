---
jurisdiction: bg
version: 1
created: 2026-11-19
instruments:
  - name: Directive 2002/58/EC
    effective: 2002-07-31
    provision: "Art. 13"
    source-url: https://eur-lex.europa.eu/eli/dir/2002/58/oj
  - name: "Закон за електронната търговия (Electronic Commerce Act)"
    provision: "чл. 6"
  - name: "Закон за електронните съобщения (Electronic Communications Act)"
marketing-exceptions: []
fixtures:
  - finding: objection_route
    triggers: bg/fixtures/advertising-without-objection-route.md
    clears: bg/fixtures/advertising-with-objection-route.md
---

# What Bulgarian law requires of an outbound message

This layer sits on `regional/eu.md` and may only tighten it. It rests on art. 6 of the Electronic
Commerce Act (Закон за електронната търговия) and on the Electronic Communications Act (Закон за
електронните съобщения), which carry Art. 13 into Bulgarian law.

## Advertising to a company

This layer asks for consent for every recipient, consumer or company. That is what the EU layer
already applies.

## The existing-customer exception

This layer states no exception. The text this layer was written against does not show that Bulgarian
law carries the existing-customer exception with all four conditions. Until a review confirms it,
the stricter reading holds and consent is the only route.

## Disclosures

The EU layer's disclosures apply. Art. 6 also asks that an unsolicited commercial message is marked
as such; the engine has no marking to check for Bulgaria.

## Where this layer takes the stricter reading

- The existing-customer exception is left out until a review confirms the Bulgarian text.
- The article that sets the marking of a commercial message must be checked by the review.

## Review

This layer is a draft. It states no `reviewed` or `reviewer`, so no citation is shown to a rep until
a legal review stamps it.
