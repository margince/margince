---
jurisdiction: hu
version: 1
created: 2026-11-19
instruments:
  - name: "2008. évi XLVIII. törvény a gazdasági reklámtevékenység alapvető feltételeiről és egyes korlátairól"
    provision: "6. §"
    source-url: https://njt.hu/jogszabaly/2008-48-00-00
  - name: "2001. évi CVIII. törvény az elektronikus kereskedelmi szolgáltatások, valamint az információs társadalommal összefüggő szolgáltatások egyes kérdéseiről"
    provision: "14. §"
marketing-exceptions: []
fixtures:
  - finding: objection_route
    triggers: hu/fixtures/advertising-without-objection-route.md
    clears: hu/fixtures/advertising-with-objection-route.md
citations:
  - code: no_marketing_consent
    territorial-test: any_sender
    instrument: "2008. évi XLVIII. törvény a gazdasági reklámtevékenység alapvető feltételeiről és egyes korlátairól"
    provision: "6. § (1)"
  - code: marketing_permission_required
    territorial-test: any_sender
    instrument: "2008. évi XLVIII. törvény a gazdasági reklámtevékenység alapvető feltételeiről és egyes korlátairól"
    provision: "6. § (1)"
  - code: marketing_consent_required
    territorial-test: any_sender
    instrument: "2008. évi XLVIII. törvény a gazdasági reklámtevékenység alapvető feltételeiről és egyes korlátairól"
    provision: "6. § (1)"
  - code: unconfirmed_double_opt_in
    territorial-test: any_sender
    instrument: "2008. évi XLVIII. törvény a gazdasági reklámtevékenység alapvető feltételeiről és egyes korlátairól"
    provision: "6. § (1)"
  - code: public_opt_out_link
    territorial-test: any_sender
    instrument: "Directive 2002/58/EC"
    provision: "Art. 13(4)"
---

# What Hungarian law requires of an outbound message

This layer sits on `regional/eu.md` and may only tighten it. It rests on § 6 of Act XLVIII of 2008
on business advertising (Grt.) and § 14 of Act CVIII of 2001 on electronic commerce (Ekertv.).

## Advertising to a company

§ 6 Grt. asks for prior, clear and specific consent before direct advertising to a natural person.
This pack asks for consent for every address, as the EU layer already does.

## The existing-customer exception

This layer states no exception. § 6 Grt. names no existing-customer exception in the text this layer
was written against, so consent is the only route.

## Disclosures

The EU layer's disclosures apply.

## Where this layer takes the stricter reading

- The existing-customer exception is left out. A review may find that Hungarian law carries one.

## Review

This layer is a draft. It states no `reviewed` or `reviewer`, so no citation is shown to a rep until
a legal review stamps it.
