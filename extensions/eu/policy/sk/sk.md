---
jurisdiction: sk
version: 1
created: 2026-11-19
instruments:
  - name: "Zákon č. 452/2021 Z. z. o elektronických komunikáciách"
    provision: "§ 116"
    source-url: https://www.slov-lex.sk/ezbierky/pravne-predpisy/SK/ZZ/2021/452/
fixtures:
  - finding: objection_route
    triggers: sk/fixtures/advertising-without-objection-route.md
    clears: sk/fixtures/advertising-with-objection-route.md
citations:
  - code: no_marketing_consent
    territorial-test: any_sender
    instrument: "Zákon č. 452/2021 Z. z. o elektronických komunikáciách"
    provision: "§ 116 ods. 3"
  - code: marketing_permission_required
    territorial-test: any_sender
    instrument: "Zákon č. 452/2021 Z. z. o elektronických komunikáciách"
    provision: "§ 116 ods. 3"
  - code: marketing_consent_required
    territorial-test: any_sender
    instrument: "Zákon č. 452/2021 Z. z. o elektronických komunikáciách"
    provision: "§ 116 ods. 3"
  - code: unconfirmed_double_opt_in
    territorial-test: any_sender
    instrument: "Zákon č. 452/2021 Z. z. o elektronických komunikáciách"
    provision: "§ 116 ods. 3"
  - code: public_opt_out_link
    territorial-test: any_sender
    instrument: "Zákon č. 452/2021 Z. z. o elektronických komunikáciách"
    provision: "§ 116 ods. 15"
---

# What Slovak law requires of an outbound message

This layer sits on `regional/eu.md` and may only tighten it. It rests on § 116 of Act No. 452/2021
Coll. on electronic communications (zákon č. 452/2021 Z. z.).

## Advertising to a company

The national rule covers legal persons as well as natural persons, so advertising to a company
address needs consent too. That is what the EU layer already applies.

## The existing-customer exception

The national act keeps the four conditions of the EU layer, so this layer inherits the exception and
adds no key for it.

## Disclosures

The EU layer's disclosures apply.

## Where this layer takes the stricter reading

- The paragraph of § 116 that holds the exception was not checked; the review must add it.

## Review

This layer is a draft. It states no `reviewed` or `reviewer`, so no citation is shown to a rep until
a legal review stamps it.
